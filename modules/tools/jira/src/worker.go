package main

// Background worker: commands queue operations and return at once, a
// detached `jira __worker` sends them to Jira one by one.

import (
	"cmp"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// spawnWorker starts a detached worker (it exits at once if one is running).
// A variable so tests can replace it.
var spawnWorker = func() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(statePath("worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(exe, "__worker")
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("не удалось запустить фоновый обработчик: %w", err)
	}
	return cmd.Process.Release()
}

func enqueue(op *Op) error {
	err := updateQueue(func(q *Queue) {
		q.Seq++
		op.ID = q.Seq
		q.Pending = append(q.Pending, op)
	})
	if err != nil {
		return err
	}
	return spawnWorker()
}

// submit runs the op now (wait) or queues it and prints a line.
func submit(op *Op, wait bool) error {
	if wait {
		return runNow(op)
	}
	if err := enqueue(op); err != nil {
		return err
	}
	fmt.Println(yellow("⏳"), "@"+strconv.Itoa(op.ID), op)
	return nil
}

// runNow executes an op in the foreground, then refreshes the cache in the background.
func runNow(op *Op) error {
	c, err := newClient()
	if err != nil {
		return err
	}
	d, err := execute(c, op)
	if err != nil {
		return err
	}
	if op.Kind == "create" {
		op.Key = d.Key // for the caller: create --switch
	}
	if op.Kind != "sync" {
		fmt.Printf("%s %s: %s\n", green("✓"), bold(d.Key), d.Result)
		cache := loadCache()
		patchIssues(c, cache, []string{d.Key})
		_ = saveCache(cache)
		_ = enqueue(&Op{Kind: "sync", Key: d.Key})
	}
	if d.Warn != "" {
		fmt.Fprintln(os.Stderr, yellow("не всё получилось:"), d.Warn)
	}
	return nil
}

func runWorker() error {
	for {
		l, err := lock("worker.lock", false)
		if err != nil {
			return nil // another worker is on it
		}
		stop := drain()
		l.Close()
		// An op may have been queued while its own worker saw our lock: look again.
		if stop || len(loadQueue().Pending) == 0 {
			return nil
		}
	}
}

// drain processes the queue; stop is true when Jira is unreachable.
func drain() (stop bool) {
	c, err := newClient()
	if err != nil {
		log.Print(err)
		return true
	}
	var touched []string // keys changed by the ops: re-read after the sync
	for {
		q := loadQueue()
		if len(q.Pending) == 0 {
			break
		}
		op := q.Pending[0]
		var d Done
		for _, wait := range []time.Duration{0, 3 * time.Second, 15 * time.Second} {
			time.Sleep(wait)
			if d, err = execute(c, op); !transient(err) {
				break
			}
		}
		if transient(err) {
			log.Printf("@%d %s: нет связи: %v", op.ID, op, err)
			_ = updateQueue(func(q *Queue) {
				for _, p := range q.Pending {
					if p.ID == op.ID {
						p.Error = err.Error()
					}
				}
			})
			return true
		}
		log.Printf("@%d %s: %s", op.ID, op, cmp.Or(errText(err), "ok "+d.Result))
		if err := finish(op, d, err); err != nil {
			log.Print(err)
			return true
		}
		if err == nil && op.Kind != "sync" && d.Key != "" {
			touched = append(touched, d.Key)
		}
	}
	if len(touched) > 0 { // so that `list` shows the new state
		if err := doSync(c, touched...); err != nil {
			log.Print("обновление кэша: ", err)
		}
	}
	return false
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return "ошибка: " + err.Error()
}

// resolveRef turns "@12" (a queued create) into the created issue key.
func resolveRef(ref string) (string, error) {
	id, err := strconv.Atoi(strings.TrimPrefix(ref, "@"))
	if !strings.HasPrefix(ref, "@") || err != nil {
		return ref, nil
	}
	if key, ok := createdKey(id); ok {
		return key, nil
	}
	return "", fmt.Errorf("%s: задача ещё не создана или создание не удалось (jira q)", ref)
}

func execute(c *Client, op *Op) (d Done, err error) {
	switch op.Kind {
	case "sync":
		return d, doSync(c, op.Key) // Key: the issue runNow just changed
	case "create":
		n := *op.Issue
		if n.Epic, err = resolveRef(n.Epic); err != nil {
			return d, err
		}
		key, warns, err := c.create(n)
		return Done{Key: key, Result: key, Warn: strings.Join(warns, "; ")}, err
	}
	if d.Key, err = resolveRef(op.Key); err != nil {
		return d, err
	}
	switch op.Kind {
	case "move":
		if d.Result, err = c.move(d.Key, op.Target, loadCache()); err == nil && op.Comment != "" {
			// The move is done: a failed comment is a warning, not a retry.
			if err := c.comment(d.Key, op.Comment); err != nil {
				d.Warn = "комментарий не добавлен: " + err.Error() + "\nтекст: " + op.Comment
			}
		}
	case "comment":
		d.Result, err = "комментарий добавлен", c.comment(d.Key, op.Comment)
	case "set":
		var done []string
		if op.Estimate != "" {
			if err = c.setEstimate(d.Key, op.Estimate); err != nil {
				return d, fmt.Errorf("оценка: %w", err)
			}
			done = append(done, "оценка "+op.Estimate)
		}
		if op.Epic != "" {
			epic := ""
			if op.Epic != noEpic {
				if epic, err = resolveRef(op.Epic); err != nil {
					return d, err
				}
			}
			if err = c.setEpic(d.Key, epic); err != nil {
				return d, fmt.Errorf("эпик: %w", err)
			}
			done = append(done, "эпик "+cmp.Or(epic, "убран"))
		}
		d.Result = strings.Join(done, ", ")
	default:
		err = fmt.Errorf("неизвестная операция %q", op.Kind)
	}
	return d, err
}

// doSync refreshes the cache: my recent issues, my open epics, board columns,
// statuses. The search index lags behind transitions, so the issues in fresh
// (just changed) are re-read directly.
func doSync(c *Client, fresh ...string) error {
	const mine = "assignee = currentUser() AND "
	fields := []string{"summary", "status", "issuetype"}
	scope := mine + "(statusCategory != Done OR updated >= -14d)"
	cache := &Cache{Updated: time.Now(), Statuses: map[string]string{}, StatusCategories: map[string]string{}}
	var issues []Issue
	var err error
	if c.Board != "" {
		if cache.Columns, err = c.columns(c.Board); err == nil {
			issues, err = c.boardIssues(c.Board, scope, fields)
		}
	} else {
		issues, err = c.search(scope+" ORDER BY updated DESC", fields, "", 300)
	}
	if err != nil {
		return err
	}
	epics, err := c.search(mine+"issuetype = Epic AND statusCategory != Done ORDER BY updated DESC", fields, "", 300)
	if err != nil {
		return err
	}
	statuses, err := c.statuses()
	if err != nil {
		return err
	}
	for _, s := range statuses {
		cache.Statuses[s.ID], cache.StatusCategories[s.ID] = s.Name, s.StatusCategory.Key
	}
	cache.Issues, cache.Epics = toItems(issues), toItems(epics)
	patchIssues(c, cache, append(fresh, recentlyChanged(10*time.Minute)...))
	return saveCache(cache)
}

// patchIssues overwrites the cached state of keys with a direct read
// (unlike search, not subject to index lag); a failed read keeps what is there.
func patchIssues(c *Client, cache *Cache, keys []string) {
	seen := map[string]bool{"": true}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		is, err := c.issue(key, "summary", "status", "issuetype")
		if err != nil {
			log.Printf("%s: %v", key, err)
			continue
		}
		fresh, found := toItems([]Issue{*is})[0], false
		for _, it := range cache.items() {
			if it.Key == key {
				id := it.ID
				*it, it.ID, found = fresh, id, true
			}
		}
		if !found {
			if cache.isEpic(fresh) {
				cache.Epics = append(cache.Epics, fresh)
			} else {
				cache.Issues = append(cache.Issues, fresh)
			}
		}
	}
}

func toItems(issues []Issue) []Item {
	out := make([]Item, len(issues))
	for i, is := range issues {
		st := is.status()
		out[i] = Item{Key: is.Key, Summary: is.Fields.Summary, Status: st.Name, StatusID: st.ID,
			Category: st.StatusCategory.Key, Type: is.Fields.IssueType.String()}
	}
	return out
}

// recentlyChanged returns the keys of issues changed by queue ops within d.
func recentlyChanged(d time.Duration) []string {
	var keys []string
	for _, done := range doneLog() {
		if time.Since(done.Time) < d {
			keys = append(keys, done.Key)
		}
	}
	return keys
}
