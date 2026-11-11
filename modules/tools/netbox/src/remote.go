package main

// Удалённая сессия без sudo: одно ssh-соединение (ControlMaster), корень хоста
// смонтирован через sshfs в ~/r/<host>, а nbsh — шим-«шелл», который выполняет
// команды на хосте, переводя локальный cwd внутри монтирования в удалённый путь.
// Программы, запущенные в сессии (nvim, …), берут $SHELL=nbsh и тем самым
// исполняют :!, :terminal, system() удалённо, а файлы читают с монтирования.

import (
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// remoteShell — `netbox shell <запрос>`.
func remoteShell(h Host, user string) {
	dest := user + "@" + h.Addr
	run := filepath.Join(env("XDG_RUNTIME_DIR", os.TempDir()), "netbox")
	os.MkdirAll(run, 0o700)
	ctl := filepath.Join(run, fmt.Sprintf("%x", sha1.Sum([]byte(dest)))[:12]+".sock")
	mnt := filepath.Join(check(os.UserHomeDir()), "r", h.Name)

	opts := []string{"-o", "StrictHostKeyChecking=accept-new", "-o", "NumberOfPasswordPrompts=1",
		"-o", "HostKeyAlias=" + h.Name, "-o", "ControlPath=" + ctl}
	ssh := func(a ...string) *exec.Cmd {
		// последний аргумент — удалённая команда ("" — без неё)
		full := append(append(append([]string{}, opts...), a[:len(a)-1]...), dest)
		if c := a[len(a)-1]; c != "" {
			full = append(full, c)
		}
		return exec.Command("ssh", full...)
	}
	owner := false // мы подняли соединение/монтирование — нам и убирать
	if ssh("-O", "check", "").Run() != nil {
		fmt.Fprintf(os.Stderr, "-> %s (%s): соединение\n", h.Name, h.Addr)
		c := ssh("-M", "-N", "-f", "-o", "ControlPersist=yes", "")
		c.Env, c.Stdin, c.Stderr = sshEnv(), os.Stdin, os.Stderr
		check(0, c.Run())
		owner = true
	}
	out, err := ssh("-T", "echo $HOME").Output()
	home := strings.TrimSpace(string(out))
	if err != nil || home == "" {
		home = "/"
	}

	if !mounted(mnt) {
		exec.Command("fusermount3", "-uz", mnt).Run() // хвост от упавшей сессии
		os.MkdirAll(mnt, 0o700)
		fmt.Fprintf(os.Stderr, "-> монтирую %s:/ в %s\n", h.Name, mnt)
		c := exec.Command("sshfs", dest+":/", mnt,
			"-o", "ControlPath="+ctl, "-o", "HostKeyAlias="+h.Name,
			"-o", "reconnect,idmap=user,transform_symlinks,dir_cache=yes")
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			if owner {
				ssh("-O", "exit", "").Run()
			}
			die("sshfs: " + err.Error())
		}
		owner = true
	}

	rc := filepath.Join(run, "bashrc")
	os.WriteFile(rc, []byte(`[ -f ~/.bashrc ] && . ~/.bashrc
# r <команда> — выполнить на хосте (без r команды идут локально)
r() { nbsh "$@"; }
`), 0o600)
	fmt.Fprintf(os.Stderr, "-> %s: файлы в %s, `r cmd` — команда на хосте, SHELL=nbsh. exit — отключиться\n", h.Name, mnt)

	sh := exec.Command("bash", "--rcfile", rc, "-i")
	sh.Dir = filepath.Join(mnt, home)
	sh.Stdin, sh.Stdout, sh.Stderr = os.Stdin, os.Stdout, os.Stderr
	nbsh, _ := exec.LookPath("nbsh")
	sh.Env = append(os.Environ(), "SHELL="+nbsh, "NB_HOST="+h.Name, "NB_ROOT="+mnt,
		"NB_DEST="+dest, "NB_CTL="+ctl, "NB_ALIAS="+h.Name)
	// сигналы ловим, а не игнорируем: SIG_IGN наследуется через exec, и тогда
	// Ctrl+C не прерывает программы в сессии. Пойманный сигнал при exec сбрасывается.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
	sh.Run()

	if owner {
		if out, err := exec.Command("fusermount3", "-u", mnt).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "netbox: %s занято (%s), отмонтирую лениво\n", mnt, strings.TrimSpace(string(out)))
			exec.Command("fusermount3", "-uz", mnt).Run()
		}
		ssh("-O", "exit", "").Run()
		os.Remove(mnt)
		fmt.Fprintf(os.Stderr, "-> %s: отключено\n", h.Name)
	}
}

func mounted(dir string) bool {
	data, _ := os.ReadFile("/proc/self/mounts")
	for _, l := range strings.Split(string(data), "\n") {
		if f := strings.Fields(l); len(f) > 1 && f[1] == dir {
			return true
		}
	}
	return false
}

// nbsh — вызывается как $SHELL: `nbsh -c 'cmd'`, `nbsh cmd args…` или `nbsh` (интерактивный шелл).
func nbsh(args []string) {
	root, dest, ctl := os.Getenv("NB_ROOT"), os.Getenv("NB_DEST"), os.Getenv("NB_CTL")
	if root == "" || dest == "" {
		die("nbsh работает только внутри `netbox shell`")
	}
	// локальные пути внутри монтирования -> удалённые
	local := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, root+"/", "/"), root, "/")
	}
	dir := "~"
	if wd, err := os.Getwd(); err == nil && (wd == root || strings.HasPrefix(wd, root+"/")) {
		dir = quote(local(wd))
	}

	var cmd string
	if len(args) >= 2 && args[0] == "-c" {
		cmd = args[1]
	} else if len(args) > 0 && args[0] != "-i" && args[0] != "-l" {
		q := make([]string, len(args))
		for i, a := range args {
			q[i] = quote(a)
		}
		cmd = strings.Join(q, " ")
	}

	tty := "-T"
	if term.IsTerminal(0) && term.IsTerminal(1) {
		tty = "-t"
	}
	remote := "cd " + dir + " 2>/dev/null || cd; "
	if cmd == "" {
		tty, remote = "-t", remote+"exec ${SHELL:-sh} -l"
	} else {
		remote += local(cmd)
	}
	a := []string{"ssh", tty, "-q", "-o", "ControlPath=" + ctl, "-o", "ControlMaster=no", "-o", "BatchMode=yes",
		"-o", "HostKeyAlias=" + os.Getenv("NB_ALIAS"), dest, remote}
	check(0, syscall.Exec(check(exec.LookPath("ssh")), a, os.Environ()))
}
