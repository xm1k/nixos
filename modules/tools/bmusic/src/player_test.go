package main

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Exercise real player commands and IPC replies without audio or the user's state.
func testPlayer(t *testing.T, handler http.HandlerFunc) (*Player, <-chan string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, server := net.Pipe()
	m := &MPV{conn: client, pending: map[int64]chan mpvResp{}, Events: make(chan mpvEvent, 256), dead: make(chan struct{})}
	loads := make(chan string, 20)
	go m.read()
	go func() {
		defer server.Close()
		sc := bufio.NewScanner(server)
		entry := int64(10)
		for sc.Scan() {
			var req struct {
				Command   []json.RawMessage `json:"command"`
				RequestID int64             `json:"request_id"`
			}
			if json.Unmarshal(sc.Bytes(), &req) != nil {
				return
			}
			var name string
			_ = json.Unmarshal(req.Command[0], &name)
			var data any
			if name == "loadfile" {
				var url string
				_ = json.Unmarshal(req.Command[1], &url)
				loads <- url
				entry++
				data = map[string]int64{"playlist_entry_id": entry}
			}
			_ = json.NewEncoder(server).Encode(map[string]any{"request_id": req.RequestID, "error": "success", "data": data})
		}
	}()
	t.Cleanup(func() { _ = client.Close() })
	p := &Player{
		api: &API{Base: srv.URL, hc: srv.Client()}, mpv: m,
		st: State{Idx: 0, Repeat: "off", Queue: []Track{
			{VideoID: "a", Duration: "3:00"}, {VideoID: "b"}, {VideoID: "c"},
			{VideoID: "d"}, {VideoID: "e"}, {VideoID: "f"},
		}},
		loaded: true, pos: 40, dur: 180, progressAt: time.Now(),
	}
	return p, loads
}

func waitLoad(t *testing.T, loads <-chan string, id string) {
	t.Helper()
	select {
	case url := <-loads:
		if !strings.HasSuffix(url, "/api/stream/"+id) {
			t.Fatalf("loaded %q, want track %s", url, id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("player did not load a track")
	}
}

func TestStallRetriesAtSavedPositionThenAdvancesPastRepeatOne(t *testing.T) {
	p, loads := testPlayer(t, nil)
	p.st.Repeat = "one"
	for i := 0; i < 3; i++ {
		p.checkProgress(p.progressAt.Add(playbackStallLimit))
		waitLoad(t, loads, "a")
		if p.seekOnLoad != 40 || p.retries != i+1 {
			t.Fatalf("retry %d: seek=%g retries=%d", i, p.seekOnLoad, p.retries)
		}
	}
	p.checkProgress(p.progressAt.Add(playbackStallLimit))
	waitLoad(t, loads, "b")
	if p.st.Idx != 1 || !p.loaded || p.paused {
		t.Fatalf("queue did not resume: %+v", p.status())
	}
}

func TestProgressAndExplicitPausePreventStallRecovery(t *testing.T) {
	p, loads := testPlayer(t, nil)
	p.progressAt = time.Now().Add(-time.Minute)
	p.handle(mpvEvent{Event: "property-change", Name: "time-pos", Data: json.RawMessage(`41`)})
	p.checkProgress(time.Now())
	p.paused = true
	p.checkProgress(p.progressAt.Add(time.Hour))
	if err := p.setPause(false); err != nil {
		t.Fatal(err)
	}
	p.checkProgress(time.Now())
	select {
	case url := <-loads:
		t.Fatalf("unexpected recovery while progressing/paused: %s", url)
	default:
	}
	p.retries, p.retryPos = 2, 40
	p.handle(mpvEvent{Event: "property-change", Name: "time-pos", Data: json.RawMessage(`71`)})
	if p.retries != 0 {
		t.Fatal("successful playback did not reset retry budget")
	}
}

func TestPlaybackErrorsAndPrematureEOFHaveBoundedRetries(t *testing.T) {
	for _, reason := range []string{"error", "eof"} {
		t.Run(reason, func(t *testing.T) {
			p, loads := testPlayer(t, nil)
			p.st.Repeat = "one"
			for i := 0; i < 4; i++ {
				p.handle(mpvEvent{Event: "end-file", Reason: reason, Error: "loading failed", Entry: p.entry})
				id := "a"
				if i == 3 {
					id = "b"
				}
				waitLoad(t, loads, id)
			}
		})
	}
}

func TestNormalEOFRepeatsAndStaleEndEventIsIgnored(t *testing.T) {
	p, loads := testPlayer(t, nil)
	p.st.Repeat, p.pos, p.entry = "one", 180, 10
	p.handle(mpvEvent{Event: "end-file", Reason: "eof", Entry: 9})
	if p.retries != 0 {
		t.Fatal("stale entry triggered recovery")
	}
	p.handle(mpvEvent{Event: "end-file", Reason: "eof", Entry: 10})
	waitLoad(t, loads, "a")
	if p.st.Idx != 0 || p.pos != 0 {
		t.Fatal("normal end no longer repeats the track")
	}
}

func TestErrorWhilePausedWaitsForExplicitResume(t *testing.T) {
	p, loads := testPlayer(t, nil)
	p.paused = true
	p.handle(mpvEvent{Event: "end-file", Reason: "error", Error: "loading failed"})
	p.checkProgress(time.Now().Add(time.Hour))
	select {
	case url := <-loads:
		t.Fatalf("error resumed a paused track: %s", url)
	default:
	}
	if err := p.setPause(false); err != nil {
		t.Fatal(err)
	}
	waitLoad(t, loads, "a")
	if p.seekOnLoad != 40 {
		t.Fatal("explicit resume lost the saved position")
	}
}

func TestQueueEndWaitsForInFlightWave(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	p, loads := testPlayer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/wave" {
			close(started)
			<-release
			_, _ = w.Write([]byte(`{"tracks":[{"videoId":"b"},{"videoId":"c"},{"videoId":"d"},{"videoId":"e"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	p.st.Queue, p.st.Wave = p.st.Queue[:1], "mix"
	go p.extend()
	<-started
	p.mu.Lock()
	p.loaded = false
	_ = p.next(true)
	p.mu.Unlock()
	// A joining request must wait for the existing network request to finish.
	joined := make(chan struct{})
	go func() { p.waveMore(); close(joined) }()
	select {
	case <-joined:
		close(release)
		t.Fatal("queue extension returned before the in-flight request")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	waitLoad(t, loads, "b")
	<-joined
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.st.Idx != 1 || !p.loaded {
		t.Fatal("new wave tracks were appended without starting playback")
	}
}
