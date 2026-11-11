package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

const usage = `netbox — поиск хостов из NetBox-инвентаря и ssh без пароля (пароль из $WORK_PASS).

  netbox [ssh] [-n] [-u user] <запрос...> [-- команда]   зайти под sudo -i (-n без sudo)
  --infra (у ssh/exec)                                   под пользователем infra, пароль из $INFRA_PASS
  netbox ls [-s] [запрос...]                             найти хосты (-s только имена)
  netbox show <запрос...>                                переменные хоста
  netbox exec [-y] [-u user] <запрос...> -- команда      команда на всех найденных
  netbox update [файл.yaml]                              обновить кеш (по умолчанию из artifactory)
  netbox completion                                      bash completion

Запрос — подстроки имени/IP или точное имя группы, должны совпасть все.
Пользователь: -u, $WORK_ADMIN_USER, $USER. Кеш обновляется сам раз в $NB_CACHE_TTL ч (24).
`

var (
	snapshotURL = env("NB_SNAPSHOT_URL", "https://artifactory-nn.atmosphere.yadro.com/artifactory/re-infra-misc/inventory/inventory_latest.yaml")
	cacheFile   = filepath.Join(check(os.UserCacheDir()), "netbox-cli", "hosts.json")
	passVar     = "WORK_PASS" // переменная окружения с паролем (--infra -> INFRA_PASS)
)

type Host struct {
	Name   string         `json:"name"`
	Addr   string         `json:"addr"`
	Groups []string       `json:"groups"`
	Vars   map[string]any `json:"vars"`
}

type node struct {
	Hosts    map[string]map[string]any `yaml:"hosts"`
	Children map[string]node           `yaml:"children"`
}

func main() {
	if os.Getenv("NB_ASKPASS") != "" { // ssh вызывает нас как SSH_ASKPASS
		if p := strings.ToLower(strings.Join(os.Args[1:], " ")); !strings.Contains(p, "yes/no") && !strings.Contains(p, "passphrase") {
			fmt.Println(os.Getenv(os.Getenv("NB_ASKPASS")))
			return
		}
		os.Exit(1)
	}

	args, tail := os.Args[1:], []string(nil)
	if i := slices.Index(args, "--"); i >= 0 {
		args, tail = args[:i], args[i+1:]
	}
	cmd := "ls"
	if len(args) > 0 {
		cmd = args[0]
		switch cmd {
		case "ls", "show", "ssh", "exec", "update", "completion":
			args = args[1:]
		case "-h", "--help", "help":
			fmt.Print(usage)
			return
		default:
			cmd = "ssh"
		}
	}
	var q []string
	flag := map[string]bool{}
	user := env("WORK_ADMIN_USER", os.Getenv("USER"))
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--infra":
			user, passVar = "infra", "INFRA_PASS"
		case a == "-u" && i+1 < len(args):
			i++
			user = args[i]
		case len(a) == 2 && a[0] == '-':
			flag[a] = true
		default:
			q = append(q, a)
		}
	}

	switch cmd {
	case "update":
		src := ""
		if len(q) > 0 {
			src = q[0]
		}
		hs := check(update(src))
		fmt.Fprintf(os.Stderr, "netbox: %d хостов -> %s\n", len(hs), cacheFile)
	case "completion":
		fmt.Println(`_netbox() { COMPREPLY=($(compgen -W "$( ((COMP_CWORD == 1)) && echo ls show ssh exec update) $(netbox ls -s 2>/dev/null)" -- "${COMP_WORDS[COMP_CWORD]}")); }
complete -F _netbox netbox`)
	case "ls":
		hs := search(q)
		if flag["-s"] {
			for _, h := range hs {
				fmt.Println(h.Name)
			}
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, h := range hs {
			fmt.Fprintf(w, "%s\t%s\t%s\n", h.Name, h.Addr, groups(h))
		}
		w.Flush()
	case "show":
		h := pick(search(q))
		out, _ := yaml.Marshal(h.Vars)
		fmt.Printf("%s\naddr: %s\ngroups: %s\n%s", h.Name, h.Addr, strings.Join(h.Groups, ", "), out)
	case "ssh":
		h := pick(search(q))
		ssh := sshArgs(h, user, "-t")
		if flag["-n"] {
			fmt.Fprintf(os.Stderr, "-> %s (%s)\n", h.Name, h.Addr)
			path := check(exec.LookPath("ssh"))
			check(0, syscall.Exec(path, append(append([]string{"ssh"}, ssh...), tail...), sshEnv()))
		}
		fmt.Fprintf(os.Stderr, "-> %s (%s) [sudo]\n", h.Name, h.Addr)
		// sshd с командой не показывает motd (как при обычном входе) — выводим сами
		remote := "cat /run/motd.dynamic /etc/motd 2>/dev/null; exec sudo -p NBSUDO: -i"
		if len(tail) > 0 {
			remote = "exec sudo -p NBSUDO: sh -c " + quote(strings.Join(tail, " "))
		}
		os.Exit(sudo(append(ssh, remote)))
	case "exec":
		hs := search(q)
		if len(hs) == 0 || len(tail) == 0 {
			die("нужен запрос и команда после --")
		}
		if !flag["-y"] {
			for _, h := range hs {
				fmt.Println(h.Name)
			}
			fmt.Printf("выполнить на %d хостах? [y/N] ", len(hs))
			var ans string
			fmt.Scanln(&ans)
			if ans != "y" && ans != "д" {
				os.Exit(1)
			}
		}
		res, sem, wg := make([]string, len(hs)), make(chan struct{}, 10), sync.WaitGroup{}
		for i, h := range hs {
			wg.Go(func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				c := exec.Command("ssh", append(sshArgs(h, user, "-T"), strings.Join(tail, " "))...)
				c.Env = sshEnv()
				out, err := c.CombinedOutput()
				status := "ok"
				if err != nil {
					status = err.Error()
				}
				res[i] = fmt.Sprintf("%s | %s\n%s", h.Name, status, out)
			})
		}
		wg.Wait()
		fmt.Print(strings.Join(res, ""))
	}
}

// update скачивает снапшот (или читает файл), разворачивает дерево групп и пишет кеш.
func update(src string) ([]Host, error) {
	var data []byte
	var err error
	if src != "" {
		data, err = os.ReadFile(src)
	} else {
		fmt.Fprintln(os.Stderr, "netbox: скачиваю", snapshotURL)
		// корпоративный self-signed CA — сертификат не проверяем
		c := http.Client{Timeout: time.Minute, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
		var r *http.Response
		if r, err = c.Get(snapshotURL); err == nil {
			defer r.Body.Close()
			if r.StatusCode != 200 {
				return nil, fmt.Errorf("%s: %s", snapshotURL, r.Status)
			}
			data, err = io.ReadAll(r.Body)
		}
	}
	if err != nil {
		return nil, err
	}
	var root map[string]node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	byName := map[string]*Host{}
	var walk func(name string, n node, path []string)
	walk = func(name string, n node, path []string) {
		path = append(slices.Clone(path), name)
		for hn, vars := range n.Hosts {
			h := byName[hn]
			if h == nil {
				h = &Host{Name: hn, Vars: map[string]any{}}
				byName[hn] = h
			}
			for k, v := range vars {
				h.Vars[k] = v
			}
			h.Groups = append(h.Groups, path...)
		}
		for cn, c := range n.Children {
			walk(cn, c, path)
		}
	}
	for name, n := range root {
		walk(name, n, nil)
	}
	var hs []Host
	for _, h := range byName {
		slices.Sort(h.Groups)
		h.Groups = slices.Compact(h.Groups)
		h.Addr = h.Name
		if a, ok := h.Vars["ansible_host"].(string); ok && a != "" {
			h.Addr = a
		}
		hs = append(hs, *h)
	}
	slices.SortFunc(hs, func(a, b Host) int { return strings.Compare(a.Name, b.Name) })
	out, _ := json.Marshal(hs)
	os.MkdirAll(filepath.Dir(cacheFile), 0o700)
	return hs, os.WriteFile(cacheFile, out, 0o600)
}

func load() []Host {
	ttl := time.Duration(24) * time.Hour
	if v := os.Getenv("NB_CACHE_TTL"); v != "" {
		d, _ := time.ParseDuration(v + "h")
		ttl = d
	}
	st, err := os.Stat(cacheFile)
	if err != nil || (ttl > 0 && time.Since(st.ModTime()) > ttl) {
		hs, uerr := update("")
		if uerr == nil {
			return hs
		}
		if err != nil {
			die("не удалось получить инвентарь: " + uerr.Error())
		}
		fmt.Fprintln(os.Stderr, "netbox: не удалось обновить кеш, использую старый:", uerr)
	}
	var hs []Host
	check(0, json.Unmarshal(check(os.ReadFile(cacheFile)), &hs))
	return hs
}

func search(q []string) []Host {
	return slices.DeleteFunc(load(), func(h Host) bool {
		for _, w := range q {
			w = strings.ToLower(w)
			if !strings.Contains(strings.ToLower(h.Name), w) && !strings.Contains(h.Addr, w) &&
				!slices.Contains(h.Groups, w) {
				return true
			}
		}
		return false
	})
}

// groups — группы без служебного шума, для показа рядом с хостом.
func groups(h Host) string {
	return strings.Join(slices.DeleteFunc(slices.Clone(h.Groups), func(g string) bool {
		for _, p := range []string{"all", "ungrouped", "status_", "is_virtual", "device_types_", "tags_atmosphere"} {
			if strings.HasPrefix(g, p) {
				return true
			}
		}
		return false
	}), ",")
}

func pick(hs []Host) Host {
	switch len(hs) {
	case 0:
		die("ничего не найдено")
	case 1:
		return hs[0]
	}
	var in bytes.Buffer
	for i, h := range hs {
		fmt.Fprintf(&in, "%d\t%s\t%s\t%s\n", i, h.Name, h.Addr, groups(h))
	}
	c := exec.Command("fzf", "--height=40%", "--reverse", "--delimiter=\t", "--with-nth=2..")
	c.Stdin, c.Stderr = &in, os.Stderr
	out, err := c.Output()
	if err != nil {
		os.Exit(130)
	}
	var i int
	fmt.Sscan(string(out), &i)
	return hs[i]
}

func sshArgs(h Host, user string, opts ...string) []string {
	return append(opts, "-o", "StrictHostKeyChecking=accept-new", "-o", "NumberOfPasswordPrompts=1",
		"-o", "HostKeyAlias="+h.Name, user+"@"+h.Addr)
}

func sshEnv() []string {
	if os.Getenv(passVar) == "" {
		fmt.Fprintf(os.Stderr, "netbox: %s пуст — ssh спросит пароль сам\n", passVar)
		return os.Environ()
	}
	return append(os.Environ(), "SSH_ASKPASS="+check(os.Executable()), "SSH_ASKPASS_REQUIRE=force", "NB_ASKPASS="+passVar)
}

// sudo запускает ssh в pty и один раз отвечает паролем на промпт sudo (маркер NBSUDO:).
func sudo(args []string) int {
	c := exec.Command("ssh", args...)
	c.Env = sshEnv()
	f := check(pty.Start(c))
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			pty.InheritSize(os.Stdin, f)
		}
	}()
	winch <- syscall.SIGWINCH
	if old, err := term.MakeRaw(0); err == nil {
		defer term.Restore(0, old)
	}
	go io.Copy(f, os.Stdin)
	marker, sent, buf := []byte("NBSUDO:"), false, make([]byte, 32<<10)
	for {
		n, err := f.Read(buf)
		chunk := buf[:n]
		if bytes.Contains(chunk, marker) {
			chunk = bytes.ReplaceAll(chunk, marker, []byte("[sudo] password: "))
			if !sent { // неверный пароль не повторяем — дальше вводится руками
				f.WriteString(os.Getenv(passVar) + "\n")
				sent = true
			}
		}
		os.Stdout.Write(chunk)
		if err != nil {
			break
		}
	}
	c.Wait()
	return c.ProcessState.ExitCode()
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "netbox:", msg)
	os.Exit(1)
}

func check[T any](v T, err error) T {
	if err != nil {
		die(err.Error())
	}
	return v
}
