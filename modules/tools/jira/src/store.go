package main

// Local state: the operation queue (~/.local/state/jira-cli) and the cache of
// my issues and the board (~/.cache/jira-cli/cache.json).

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func stateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "jira-cli")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "jira-cli")
}

func statePath(name string) string { return filepath.Join(stateDir(), name) }

func cachePath() string {
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, "jira-cli", "cache.json")
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// writeJSON writes atomically: a reader never sees a half-written file.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", data, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// lock takes an exclusive flock on a file in the state dir; Close releases it.
func lock(name string, wait bool) (*os.File, error) {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(statePath(name), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_EX
	if !wait {
		how |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// ---- queue --------------------------------------------------------------------

const noEpic = "none" // Op.Epic that takes the issue out of its epic

type Op struct {
	ID             int
	Kind           string    // create | move | set | comment | sync
	Key            string    `json:",omitempty"` // issue key or @id of a queued create
	Summary        string    `json:",omitempty"` // issue title, for display
	Target         string    `json:",omitempty"` // move: column / status / transition
	Comment        string    `json:",omitempty"` // comment text; move: added after the transition
	Estimate, Epic string    `json:",omitempty"` // set
	FixVersion     string    `json:",omitempty"` // set; with Epic none, "" is the nearest version
	Issue          *NewIssue `json:",omitempty"` // create
	Error          string    `json:",omitempty"` // last error
}

// issueLabel is key with the cached issue title: RE-1 «Title».
func issueLabel(cache *Cache, key string) string {
	if is := cache.find(key); is != nil && is.Summary != "" {
		return key + " «" + clip(is.Summary, 50) + "»"
	}
	return key
}

// label is the issue key with its title, when known.
func (o *Op) label() string {
	if o.Summary == "" {
		return o.Key
	}
	return o.Key + " «" + clip(o.Summary, 50) + "»"
}

func (o *Op) String() string {
	switch o.Kind {
	case "create":
		return "создать «" + o.Issue.Summary + "»"
	case "move":
		s := o.label() + " → " + o.Target
		if o.Comment != "" {
			s += " (+комментарий)"
		}
		return s
	case "comment":
		return o.label() + ": комментарий «" + clip(o.Comment, 40) + "»"
	case "set":
		var parts []string
		if o.Estimate != "" {
			parts = append(parts, "оценка "+o.Estimate)
		}
		if o.Epic == noEpic {
			parts = append(parts, "без эпика")
		} else if o.Epic != "" {
			parts = append(parts, "эпик "+o.Epic)
		}
		if o.FixVersion != "" {
			parts = append(parts, "fix version "+o.FixVersion)
		}
		return o.label() + ": " + strings.Join(parts, ", ")
	}
	return "обновить кэш"
}

type Queue struct {
	Seq             int
	Pending, Failed []*Op
}

func loadQueue() *Queue {
	q := &Queue{}
	if readJSON(statePath("queue.json"), q) != nil {
		data, _ := os.ReadFile(statePath("seq")) // ids continue those of older versions
		q.Seq, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	return q
}

// updateQueue changes the queue under the state lock.
func updateQueue(fn func(*Queue)) error {
	l, err := lock("state.lock", true)
	if err != nil {
		return err
	}
	defer l.Close()
	q := loadQueue()
	fn(q)
	return writeJSON(statePath("queue.json"), q)
}

func without(ops []*Op, id int) []*Op {
	out := ops[:0:0]
	for _, op := range ops {
		if op.ID != id {
			out = append(out, op)
		}
	}
	return out
}

// Done is a line of done.jsonl, the log of completed operations.
type Done struct {
	ID                            int
	Kind, What, Key, Result, Warn string
	Time                          time.Time
}

func doneLog() []Done {
	f, err := os.Open(statePath("done.jsonl"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var log []Done
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var d Done
		if json.Unmarshal(sc.Bytes(), &d) == nil {
			log = append(log, d)
		}
	}
	return log
}

// finish removes the op from the queue: logged as done, or kept in Failed.
func finish(op *Op, d Done, cause error) error {
	return updateQueue(func(q *Queue) {
		q.Pending = without(q.Pending, op.ID)
		if cause != nil {
			op.Error = cause.Error()
			q.Failed = append(q.Failed, op)
		} else if op.Kind != "sync" {
			d.ID, d.Kind, d.What, d.Time = op.ID, op.Kind, op.String(), time.Now()
			if f, err := os.OpenFile(statePath("done.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
				_ = json.NewEncoder(f).Encode(d)
				f.Close()
			}
		}
	})
}

// createdKey returns the key of the issue created by queued op id.
func createdKey(id int) (string, bool) {
	log := doneLog()
	for i := len(log) - 1; i >= 0; i-- {
		if log[i].ID == id && log[i].Kind == "create" && log[i].Result != "" {
			return log[i].Result, true
		}
	}
	return "", false
}

func lastEpic() string {
	data, _ := os.ReadFile(statePath("last_epic"))
	return strings.TrimSpace(string(data))
}

func setLastEpic(key string) { _ = os.WriteFile(statePath("last_epic"), []byte(key), 0o600) }

// ---- cache --------------------------------------------------------------------

// Item is a cached issue. ID is a short number that stays the same while the
// issue is in the cache: `jira move 3`.
type Item struct {
	ID                                             int
	Key, Summary, Status, StatusID, Category, Type string
}

type Cache struct {
	Updated          time.Time
	Columns          []Column
	Statuses         map[string]string // status id → name
	StatusCategories map[string]string // status id → new | indeterminate | done
	Issues, Epics    []Item
	Versions         []Version // open versions of default_project, the nearest first
}

func loadCache() *Cache {
	c := &Cache{}
	_ = readJSON(cachePath(), c)
	return c
}

// saveCache numbers the issues: known ones keep their numbers, new ones take
// the smallest free.
func saveCache(c *Cache) error {
	l, err := lock("state.lock", true)
	if err != nil {
		return err
	}
	defer l.Close()
	prev := map[string]int{}
	for _, is := range loadCache().items() {
		prev[is.Key] = is.ID
	}
	used, ids := map[int]bool{}, map[string]int{} // an epic on the board is in both lists: one number
	var fresh []*Item
	for _, is := range c.items() {
		if id := prev[is.Key]; id > 0 && (!used[id] || ids[is.Key] == id) {
			is.ID, used[id], ids[is.Key] = id, true, id
		} else {
			fresh = append(fresh, is)
		}
	}
	next := 1
	for _, is := range fresh {
		if id := ids[is.Key]; id > 0 {
			is.ID = id
			continue
		}
		for used[next] {
			next++
		}
		is.ID, used[next], ids[is.Key] = next, true, next
	}
	return writeJSON(cachePath(), c)
}

func (c *Cache) items() []*Item {
	var out []*Item
	for i := range c.Issues {
		out = append(out, &c.Issues[i])
	}
	for i := range c.Epics {
		out = append(out, &c.Epics[i])
	}
	return out
}

func (c *Cache) find(key string) *Item {
	for _, is := range c.items() {
		if is.Key == key {
			return is
		}
	}
	return nil
}

func (c *Cache) byID(id int) *Item {
	for _, is := range c.items() {
		if is.ID == id {
			return is
		}
	}
	return nil
}

func (c *Cache) isEpic(is Item) bool {
	if strings.EqualFold(is.Type, "Epic") || strings.EqualFold(is.Type, "Эпик") {
		return true
	}
	for _, e := range c.Epics {
		if e.Key == is.Key {
			return true
		}
	}
	return false
}

// columnOf returns the board column of a status id, -1 if none.
func (c *Cache) columnOf(statusID string) int {
	for i, col := range c.Columns {
		for _, s := range col.Statuses {
			if s.ID == statusID {
				return i
			}
		}
	}
	return -1
}

// columnIndex finds a column by name (exact, then unique prefix) or by the
// name of one of its statuses; -1 if none.
func (c *Cache) columnIndex(q string) int {
	q = strings.ToLower(strings.TrimSpace(q))
	prefix := -1
	for i, col := range c.Columns {
		name := strings.ToLower(col.Name)
		if name == q {
			return i
		}
		if q != "" && strings.HasPrefix(name, q) {
			if prefix == -1 {
				prefix = i
			} else {
				prefix = -2 // ambiguous
			}
		}
	}
	if prefix >= 0 {
		return prefix
	}
	for i, col := range c.Columns {
		for _, s := range col.Statuses {
			if q != "" && strings.ToLower(c.Statuses[s.ID]) == q {
				return i
			}
		}
	}
	return -1
}

// closing reports whether a column holds a Done-category status: moving
// there requires a comment.
func (c *Cache) closing(target string) bool {
	if i := c.columnIndex(target); i >= 0 {
		for _, s := range c.Columns[i].Statuses {
			if c.StatusCategories[s.ID] == "done" {
				return true
			}
		}
		return false
	}
	for id, name := range c.Statuses {
		if strings.EqualFold(name, target) && c.StatusCategories[id] == "done" {
			return true
		}
	}
	return false
}
