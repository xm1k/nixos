// Command jira works with my issues in Jira Server/DC: an instant list from a
// local cache, create/move/comment/set queued for a background worker, and an
// activity report for the week.
package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:           "jira",
		Short:         "Jira в терминале: мои задачи, фоновая очередь, отчёт за неделю",
		Long:          "create, move, set и comment ставятся в очередь, её разбирает фоновый обработчик.\nБез аргументов — то же, что `jira list`.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          func(*cobra.Command, []string) error { return printList(false, "") },
	}
	root.AddCommand(listCmd(), currentCmd(), createCmd(), moveCmd(), setCmd(), commentCmd(), viewCmd(),
		epicsCmd(), reportCmd(), queueCmd(), syncCmd(), initCmd(),
		&cobra.Command{Use: "__worker", Hidden: true, RunE: func(*cobra.Command, []string) error { return runWorker() }})
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(1)
	}
}

// ---- terminal -------------------------------------------------------------------

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

var useColor = os.Getenv("NO_COLOR") == "" && isTTY(os.Stdout)

func paint(code string) func(string) string {
	return func(s string) string {
		if !useColor || s == "" {
			return s
		}
		return "\x1b[" + code + "m" + s + "\x1b[0m"
	}
}

var (
	bold, dim     = paint("1"), paint("2")
	red, green    = paint("31"), paint("32")
	yellow, blue  = paint("33"), paint("34")
	errCancelled  = errors.New("отменено")
	estimateRe    = regexp.MustCompile(`^(\d+(\.\d+)?\s*[wdhm]\s*)+$`)
	issueNumberRe = regexp.MustCompile(`^\d+$`)
)

func statusPaint(name, category string) string {
	switch category {
	case "done":
		return green(name)
	case "indeterminate":
		return yellow(name)
	}
	return blue(name)
}

func interactive() bool { return isTTY(os.Stdin) && isTTY(os.Stderr) }

// gum runs a gum prompt (input "" = the terminal); Esc / Ctrl-C cancel.
func gum(input string, args ...string) (string, error) {
	cmd := exec.Command("gum", args...)
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "", errCancelled
	}
	return strings.TrimSpace(string(out)), err
}

func ask(header, placeholder string) (string, error) {
	return gum("", "input", "--header", header, "--placeholder", placeholder)
}

func askText(header string) (string, error) {
	return gum("", "write", "--header", header+"\nEnter — готово · Ctrl+J — новая строка · Ctrl+E — $EDITOR",
		"--width", "80", "--height", "8")
}

func choose(header string, options []string) (int, error) {
	if len(options) == 0 {
		return -1, errors.New("не из чего выбирать")
	}
	out, err := gum(strings.Join(options, "\n"), "filter", "--header", header, "--height", "15")
	for i, o := range options {
		if err == nil && o == out {
			return i, nil
		}
	}
	return -1, cmp.Or(err, errCancelled)
}

// ---- arguments ------------------------------------------------------------------

// issueKey normalizes an issue reference: a number from `jira list` ("3"), a
// bare issue number ("47522" → RE-47522), a key, or "@12" (a queued create).
func issueKey(arg string) string {
	arg = strings.ToUpper(strings.TrimSpace(arg))
	if !issueNumberRe.MatchString(arg) {
		return arg
	}
	n, _ := strconv.Atoi(arg)
	if is := loadCache().byID(n); is != nil {
		return is.Key
	}
	if cfg, _ := loadConfig(); cfg != nil && cfg.Project != "" {
		return strings.ToUpper(cfg.Project) + "-" + arg
	}
	return arg
}

// textArg joins args; "-" reads stdin.
func textArg(args ...string) (string, error) {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "-" {
		data, err := io.ReadAll(os.Stdin)
		return strings.TrimSpace(string(data)), err
	}
	return text, nil
}

func validEstimate(s string) error {
	if s != "" && !estimateRe.MatchString(s) {
		return fmt.Errorf("оценка «%s» не похожа на длительность Jira: 30m, 2h, 1d 4h, 1w", s)
	}
	return nil
}

// hidden issues are left out of lists and pickers unless -a: closed ones and
// Verified (in progress on the board, but finished for me).
func hidden(is Item) bool {
	return is.Category == "done" || strings.EqualFold(is.Status, "Verified")
}

func clip(s string, n int) string {
	if r := []rune(strings.Join(strings.Fields(s), " ")); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return strings.Join(strings.Fields(s), " ")
}
