package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// MPV drives an mpv process over its JSON IPC socket.
type MPV struct {
	cmd     *exec.Cmd
	conn    net.Conn
	wmu     sync.Mutex
	pmu     sync.Mutex
	nextID  int64
	pending map[int64]chan mpvResp
	Events  chan mpvEvent
	dead    chan struct{}
}

type mpvResp struct {
	Error string          `json:"error"`
	Data  json.RawMessage `json:"data"`
}

type mpvEvent struct {
	Event  string          `json:"event"`
	Name   string          `json:"name"`
	Data   json.RawMessage `json:"data"`
	Reason string          `json:"reason"`
	ID     int64           `json:"id"`
	Error  string          `json:"file_error"`
	Entry  int64           `json:"playlist_entry_id"`
}

func StartMPV(token string, volume float64) (*MPV, error) {
	sock := mpvSocket()
	_ = os.Remove(sock)
	bin := os.Getenv("BMUSIC_MPV")
	if bin == "" {
		bin = "mpv"
	}
	args := []string{
		"--idle=yes", "--no-video", "--no-terminal", "--no-config",
		"--input-ipc-server=" + sock,
		"--audio-display=no", "--keep-open=no",
		fmt.Sprintf("--volume=%g", volume),
		"--cache=yes", "--demuxer-max-bytes=64MiB",
		// ffmpeg reconnects when the stream stalls instead of reporting a premature EOF
		"--stream-lavf-o=reconnect=1,reconnect_streamed=1,reconnect_on_network_error=1,reconnect_delay_max=5",
		"--user-agent=bmusic",
		"--title=bmusic",
	}
	// extra mpv options, e.g. BMUSIC_MPV_OPTS="--audio-device=pipewire/xyz"
	args = append(args, strings.Fields(os.Getenv("BMUSIC_MPV_OPTS"))...)
	if token != "" {
		args = append(args, "--http-header-fields=Cookie: bb_sess="+token)
	}
	cmd := exec.Command(bin, args...)
	// a foreign LD_LIBRARY_PATH (e.g. another nixpkgs' alsa-lib) breaks mpv's own libraries
	env := cleanEnv()
	for i := 0; i < len(env); i++ {
		if strings.HasPrefix(env[i], "LD_LIBRARY_PATH=") {
			env = append(env[:i], env[i+1:]...)
			i--
		}
	}
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("не удалось запустить mpv: %w", err)
	}
	m := &MPV{cmd: cmd, pending: map[int64]chan mpvResp{}, Events: make(chan mpvEvent, 256), dead: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(m.dead) }()

	var conn net.Conn
	var err error
	for i := 0; i < 50; i++ {
		conn, err = net.Dial("unix", sock)
		if err == nil {
			break
		}
		select {
		case <-m.dead:
			return nil, errors.New("mpv завершился при запуске")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("mpv ipc: %w", err)
	}
	m.conn = conn
	go m.read()
	for i, p := range []string{"time-pos", "pause", "duration", "volume"} {
		if _, err := m.Command("observe_property", i+1, p); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *MPV) read() {
	sc := bufio.NewScanner(m.conn)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Bytes()
		var probe struct {
			RequestID *int64 `json:"request_id"`
			Event     string `json:"event"`
		}
		if json.Unmarshal(line, &probe) != nil {
			continue
		}
		if probe.Event != "" {
			var ev mpvEvent
			if json.Unmarshal(line, &ev) == nil {
				if ev.Event == "property-change" {
					select {
					case m.Events <- ev:
					default: // frequent and superseded by the next one: fine to drop
					}
				} else {
					select {
					case m.Events <- ev:
					case <-time.After(3 * time.Second):
					}
				}
			}
			continue
		}
		if probe.RequestID != nil {
			var r mpvResp
			_ = json.Unmarshal(line, &r)
			m.pmu.Lock()
			ch := m.pending[*probe.RequestID]
			delete(m.pending, *probe.RequestID)
			m.pmu.Unlock()
			if ch != nil {
				ch <- r
			}
		}
	}
	m.pmu.Lock()
	for id, ch := range m.pending {
		close(ch)
		delete(m.pending, id)
	}
	m.pmu.Unlock()
	close(m.Events)
}

func (m *MPV) Alive() bool {
	select {
	case <-m.dead:
		return false
	default:
		return true
	}
}

func (m *MPV) Command(args ...any) (json.RawMessage, error) {
	m.pmu.Lock()
	m.nextID++
	id := m.nextID
	ch := make(chan mpvResp, 1)
	m.pending[id] = ch
	m.pmu.Unlock()

	b, _ := json.Marshal(map[string]any{"command": args, "request_id": id})
	m.wmu.Lock()
	_, err := m.conn.Write(append(b, '\n'))
	m.wmu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return nil, errors.New("mpv закрылся")
		}
		if r.Error != "success" {
			return nil, fmt.Errorf("mpv %v: %s", args[0], r.Error)
		}
		return r.Data, nil
	case <-time.After(5 * time.Second):
		m.pmu.Lock()
		delete(m.pending, id)
		m.pmu.Unlock()
		return nil, errors.New("mpv не отвечает")
	}
}

func (m *MPV) Set(prop string, v any) error {
	_, err := m.Command("set_property", prop, v)
	return err
}

func (m *MPV) Quit() {
	_, _ = m.Command("quit")
	select {
	case <-m.dead:
	case <-time.After(2 * time.Second):
		_ = m.cmd.Process.Kill()
	}
	_ = m.conn.Close()
}
