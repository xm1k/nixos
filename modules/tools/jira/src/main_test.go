package main

import (
	"slices"
	"testing"
	"time"
)

func testCache() *Cache {
	col := func(name string, ids ...string) Column {
		c := Column{Name: name}
		for _, id := range ids {
			c.Statuses = append(c.Statuses, struct{ ID string }{id})
		}
		return c
	}
	return &Cache{
		Columns:          []Column{col("To Do", "1"), col("In Progress", "3"), col("Review", "4"), col("Done", "5", "6")},
		Statuses:         map[string]string{"1": "Open", "3": "In Progress", "4": "In Review", "5": "Closed", "6": "Verified"},
		StatusCategories: map[string]string{"1": "new", "3": "indeterminate", "4": "indeterminate", "5": "done", "6": "indeterminate"},
	}
}

func TestHiddenAndInProgress(t *testing.T) {
	c := testCache()
	c.Issues = []Item{
		{Key: "RE-1", Status: "In Progress", Category: "indeterminate", Type: "Task"},
		{Key: "RE-2", Status: "Verified", Category: "indeterminate"},
		{Key: "RE-3", Status: "Done", Category: "done"},
		{Key: "RE-4", Status: "In Progress", Type: "Epic"},
		{Key: "RE-5", Status: "In Progress", Type: "Task"},
	}
	c.Epics = []Item{{Key: "RE-5"}}
	var got []bool
	for _, is := range c.Issues {
		got = append(got, hidden(is))
	}
	if !slices.Equal(got, []bool{false, true, true, false, false}) {
		t.Errorf("hidden = %v", got)
	}
	if cur := inProgress(c); len(cur) != 1 || cur[0].Key != "RE-1" {
		t.Errorf("inProgress = %v", cur)
	}
}

func TestColumns(t *testing.T) {
	c := testCache()
	for q, want := range map[string]int{"review": 2, "in": 1, "verified": 3, "x": -1, "": -1} {
		if got := c.columnIndex(q); got != want {
			t.Errorf("columnIndex(%q) = %d, want %d", q, got, want)
		}
	}
	if !c.closing("done") || !c.closing("closed") || c.closing("review") {
		t.Error("closing")
	}
	ts := []Transition{{ID: "a", To: Status{ID: "3"}}, {ID: "b", To: Status{ID: "4"}}, {ID: "c", To: Status{ID: "6"}}}
	if s := nextStep(ts, c, 0, 3); s == nil || s.ID != "c" {
		t.Errorf("nextStep into target = %v", s)
	}
	if s := nextStep(ts[:2], c, 0, 3); s == nil || s.ID != "b" {
		t.Errorf("nextStep closest = %v", s)
	}
	if s := nextStep(ts[:1], c, 2, 3); s != nil {
		t.Errorf("nextStep must not go back: %v", s)
	}
}

func TestSaveCacheKeepsNumbers(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	_ = saveCache(&Cache{Issues: []Item{{Key: "A"}, {Key: "B"}}, Epics: []Item{{Key: "E"}}})
	_ = saveCache(&Cache{Issues: []Item{{Key: "C"}, {Key: "B"}, {Key: "E"}, {Key: "F"}}, Epics: []Item{{Key: "E"}, {Key: "F"}}})
	var ids []int
	for _, is := range loadCache().items() {
		ids = append(ids, is.ID)
	}
	if !slices.Equal(ids, []int{1, 2, 3, 4, 3, 4}) {
		t.Errorf("ids = %v", ids)
	}
}

func TestQueue(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	spawnWorker = func() error { return nil }
	a, b := &Op{Kind: "comment", Key: "RE-1", Comment: "x"}, &Op{Kind: "create", Issue: &NewIssue{Summary: "s"}}
	_ = enqueue(a)
	_ = enqueue(b)
	_ = finish(b, Done{Result: "RE-9"}, nil)
	if key, _ := resolveRef("@2"); key != "RE-9" {
		t.Errorf("resolveRef = %q", key)
	}
	if q := loadQueue(); len(q.Pending) != 1 || q.Pending[0].ID != 1 || q.Seq != 2 {
		t.Errorf("queue = %+v", q)
	}
}

func TestReportPeriod(t *testing.T) {
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.Local) // Thursday
	since, until, _ := reportPeriod(now, 7, false, true)
	if since.Day() != 2 || until.Day() != 9 {
		t.Errorf("last week = %v – %v", since, until)
	}
	if formatDuration(9000) != "2h 30m" || formatDuration(8*3600) != "1d" {
		t.Error("formatDuration")
	}
}
