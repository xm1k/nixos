package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func runDaemon() error {
	sock := socketPath()
	if c, err := net.Dial("unix", sock); err == nil {
		c.Close()
		return errors.New("демон уже запущен")
	}
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	_ = os.Chmod(sock, 0o600)
	log.SetFlags(log.LstdFlags)
	log.Printf("bmusic daemon on %s", sock)

	p := NewPlayer()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		_, _ = p.Handle(Request{Cmd: "quit"})
	}()

	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go serve(p, c)
	}
}

func serve(p *Player, c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(60 * time.Second))
	rd := bufio.NewReader(c)
	line, err := rd.ReadBytes('\n')
	if err != nil {
		return
	}
	var req Request
	resp := Response{OK: true}
	if err := json.Unmarshal(line, &req); err != nil {
		resp = Response{Error: err.Error()}
	} else if data, err := p.Handle(req); err != nil {
		resp = Response{Error: err.Error()}
	} else {
		resp.Data = data
	}
	b, _ := json.Marshal(resp)
	_, _ = c.Write(append(b, '\n'))
}

// ---------- client side ----------

var errNoDaemon = errors.New("плеер не запущен")

func dialDaemon(start bool) (net.Conn, error) {
	c, err := net.Dial("unix", socketPath())
	if err == nil || !start {
		if err != nil {
			return nil, errNoDaemon
		}
		return c, nil
	}
	if err := spawnDaemon(); err != nil {
		return nil, err
	}
	for i := 0; i < 50; i++ {
		if c, err = net.Dial("unix", socketPath()); err == nil {
			return c, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("демон не поднялся, см. %s", filepath.Join(stateDir(), "daemon.log"))
}

func spawnDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(stateDir(), 0o700)
	logf, err := os.OpenFile(filepath.Join(stateDir(), "daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(exe, "daemon")
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.Env = cleanEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// rpc sends one command to the daemon. start: launch it if it is not running.
func rpc(cmd string, args any, out any, start bool) error {
	c, err := dialDaemon(start)
	if err != nil {
		return err
	}
	defer c.Close()
	req := Request{Cmd: cmd}
	if args != nil {
		if req.Args, err = json.Marshal(args); err != nil {
			return err
		}
	}
	b, _ := json.Marshal(req)
	if _, err := c.Write(append(b, '\n')); err != nil {
		return err
	}
	_ = c.SetReadDeadline(time.Now().Add(90 * time.Second))
	var raw struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(c).Decode(&raw); err != nil {
		return err
	}
	if !raw.OK {
		return errors.New(raw.Error)
	}
	if out != nil && len(raw.Data) > 0 {
		return json.Unmarshal(raw.Data, out)
	}
	return nil
}

func getStatus() (*Status, error) {
	var s Status
	err := rpc("status", nil, &s, false)
	return &s, err
}

type queueResp struct {
	Queue []Track `json:"queue"`
	Idx   int     `json:"idx"`
	QVer  int     `json:"qver"`
}

func getQueue() (*queueResp, error) {
	var q queueResp
	err := rpc("queue", nil, &q, false)
	return &q, err
}
