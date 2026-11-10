package main

// Jira Server / Data Center REST client (API v2, Personal Access Token).

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	api   = "/rest/api/2"
	agile = "/rest/agile/1.0"
)

// Config lives in ~/.config/jira-cli/config.json.
type Config struct {
	URL     string `json:"url"`
	Token   string `json:"token"`
	Project string `json:"default_project"`
	Board   string `json:"default_board,omitempty"` // board id: its columns are `move` targets
}

func configPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "jira-cli", "config.json")
}

func loadConfig() (*Config, error) {
	cfg := &Config{}
	if err := readJSON(configPath(), cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", configPath(), err)
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	return cfg, nil
}

type Client struct {
	*Config
	hc http.Client
	me *User
}

func newClient() (*Client, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.URL == "" || cfg.Token == "" {
		return nil, errors.New("Jira не настроена: jira init")
	}
	return &Client{Config: cfg, hc: http.Client{Timeout: 30 * time.Second}}, nil
}

// apiError.Status 0 means Jira could not be reached.
type apiError struct {
	Status int
	Msg    string
}

func (e *apiError) Error() string { return e.Msg }

// transient reports whether a retry later may succeed (network, 5xx, 429).
func transient(err error) bool {
	var e *apiError
	return errors.As(err, &e) && (e.Status == 0 || e.Status == 429 || e.Status >= 500)
}

func (c *Client) do(method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.URL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return &apiError{Msg: "нет связи с Jira: " + err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return &apiError{Msg: "обрыв связи с Jira: " + err.Error()}
	}
	if resp.StatusCode >= 400 {
		var p struct {
			ErrorMessages []string
			Errors        map[string]string
		}
		_ = json.Unmarshal(data, &p)
		msgs := p.ErrorMessages
		for k, v := range p.Errors {
			msgs = append(msgs, k+": "+v)
		}
		sort.Strings(msgs)
		if len(msgs) == 0 {
			msgs = []string{http.StatusText(resp.StatusCode)}
		}
		return &apiError{resp.StatusCode, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.Join(msgs, "; "))}
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// ---- types (JSON field names match case-insensitively) -----------------------

type User struct{ Name, Key, DisplayName string }

func (u *User) String() string {
	if u == nil {
		return ""
	}
	return cmp.Or(u.DisplayName, u.Name)
}

func (u *User) Is(o *User) bool {
	return u != nil && o != nil && (u.Key != "" && u.Key == o.Key || u.Name != "" && u.Name == o.Name)
}

type Status struct {
	ID, Name       string
	StatusCategory struct{ Key string } // new | indeterminate | done
}

type Named struct{ Name string }

func (n *Named) String() string {
	if n == nil {
		return ""
	}
	return n.Name
}

type Comment struct {
	Author        *User
	Body, Created string
}

type Worklog struct {
	Author                      *User
	Comment, Started, TimeSpent string
	TimeSpentSeconds            int
}

type History struct {
	Author  *User
	Created string
	Items   []struct{ Field, From, To, FromString, ToString string }
}

type Issue struct {
	Key    string
	Fields struct {
		Summary, Description, Created, Updated string
		Status                                 *Status
		IssueType, Priority                    *Named
		Assignee, Reporter, Creator            *User
		Labels                                 []string
		Comment                                *struct {
			Comments []Comment
			Total    int
		}
		Worklog *struct {
			Worklogs []Worklog
			Total    int
		}
	}
	Changelog *struct {
		Histories []History
		Total     int
	}
}

func (is *Issue) status() Status {
	if is.Fields.Status == nil {
		return Status{}
	}
	return *is.Fields.Status
}

type Transition struct {
	ID, Name string
	To       Status
}

type Column struct {
	Name     string
	Statuses []struct{ ID string }
}

// NewIssue is a queued create.
type NewIssue struct {
	Project, Type, Summary, Description, Estimate, Epic string
	Labels                                              []string
}

// ---- requests -----------------------------------------------------------------

func issuePath(key string) string { return api + "/issue/" + url.PathEscape(key) }

func (c *Client) myself() (*User, error) {
	if c.me == nil {
		var u User
		if err := c.do("GET", api+"/myself", nil, &u); err != nil {
			return nil, err
		}
		c.me = &u
	}
	return c.me, nil
}

func (c *Client) issue(key string, fields ...string) (*Issue, error) {
	var is Issue
	return &is, c.do("GET", issuePath(key)+"?fields="+strings.Join(fields, ","), nil, &is)
}

func (c *Client) search(jql string, fields []string, expand string, limit int) ([]Issue, error) {
	var all []Issue
	for len(all) < limit {
		body := map[string]any{"jql": jql, "fields": fields, "startAt": len(all), "maxResults": min(100, limit-len(all))}
		if expand != "" {
			body["expand"] = []string{expand}
		}
		var page struct {
			Issues []Issue
			Total  int
		}
		if err := c.do("POST", api+"/search", body, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Issues...)
		if len(page.Issues) == 0 || len(all) >= page.Total {
			break
		}
	}
	return all, nil
}

// boardIssues returns the issues on a board matching jql, in rank order.
func (c *Client) boardIssues(board, jql string, fields []string) ([]Issue, error) {
	var all []Issue
	for len(all) < 300 {
		q := url.Values{"jql": {jql}, "fields": {strings.Join(fields, ",")},
			"startAt": {strconv.Itoa(len(all))}, "maxResults": {"100"}}
		var page struct {
			Issues []Issue
			Total  int
		}
		if err := c.do("GET", agile+"/board/"+url.PathEscape(board)+"/issue?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Issues...)
		if len(page.Issues) == 0 || len(all) >= page.Total {
			break
		}
	}
	return all, nil
}

func (c *Client) columns(board string) ([]Column, error) {
	var r struct{ ColumnConfig struct{ Columns []Column } }
	err := c.do("GET", agile+"/board/"+url.PathEscape(board)+"/configuration", nil, &r)
	return r.ColumnConfig.Columns, err
}

func (c *Client) statuses() ([]Status, error) {
	var ss []Status
	return ss, c.do("GET", api+"/status", nil, &ss)
}

func (c *Client) transitions(key string) ([]Transition, error) {
	var r struct{ Transitions []Transition }
	return r.Transitions, c.do("GET", issuePath(key)+"/transitions", nil, &r)
}

func (c *Client) transition(key, id string) error {
	return c.do("POST", issuePath(key)+"/transitions", map[string]any{"transition": map[string]string{"id": id}}, nil)
}

func (c *Client) comment(key, text string) error {
	return c.do("POST", issuePath(key)+"/comment", map[string]string{"body": text}, nil)
}

func (c *Client) setEstimate(key, estimate string) error {
	return c.do("PUT", issuePath(key), map[string]any{"fields": map[string]any{
		"timetracking": map[string]string{"originalEstimate": estimate}}}, nil)
}

// setEpic moves an issue into the epic; "" takes it out of its epic.
func (c *Client) setEpic(key, epic string) error {
	return c.do("POST", agile+"/epic/"+cmp.Or(url.PathEscape(epic), "none")+"/issue",
		map[string][]string{"issues": {key}}, nil)
}

// create creates the issue and assigns it to me; failures after the issue
// exists (assignee, estimate, epic) are warnings.
func (c *Client) create(n NewIssue) (key string, warns []string, err error) {
	fields := map[string]any{
		"project":   map[string]string{"key": n.Project},
		"issuetype": map[string]string{"name": cmp.Or(n.Type, "Task")},
		"summary":   n.Summary,
	}
	if n.Description != "" {
		fields["description"] = n.Description
	}
	if len(n.Labels) > 0 {
		fields["labels"] = n.Labels
	}
	var r struct{ Key string }
	if err := c.do("POST", api+"/issue", map[string]any{"fields": fields}, &r); err != nil {
		return "", nil, err
	}
	warn := func(what string, err error) {
		if err != nil {
			warns = append(warns, what+": "+err.Error())
		}
	}
	me, err := c.myself()
	if err == nil {
		err = c.do("PUT", issuePath(r.Key)+"/assignee", map[string]string{"name": me.Name}, nil)
	}
	warn("исполнитель", err)
	if n.Estimate != "" {
		warn("оценка", c.setEstimate(r.Key, n.Estimate))
	}
	if n.Epic != "" {
		warn("эпик", c.setEpic(r.Key, n.Epic))
	}
	return r.Key, warns, nil
}

// move transitions an issue to target: a transition, status or board column
// name. Without a direct transition into the target column the issue steps
// through the columns between the current and the target one.
func (c *Client) move(key, target string, cache *Cache) (string, error) {
	ts, err := c.transitions(key)
	if err != nil {
		return "", err
	}
	if t := findTransition(ts, target); t != nil {
		return t.To.Name, c.transition(key, t.ID)
	}
	tc := cache.columnIndex(target)
	if tc < 0 {
		names := make([]string, len(ts))
		for i, t := range ts {
			names[i] = t.Name
		}
		return "", fmt.Errorf("переход «%s» недоступен, доступно: %s", target, strings.Join(names, ", "))
	}
	is, err := c.issue(key, "status")
	if err != nil {
		return "", err
	}
	st := is.status()
	for cur := cache.columnOf(st.ID); cur != tc; cur = cache.columnOf(st.ID) {
		t := nextStep(ts, cache, cur, tc)
		if t == nil {
			return "", fmt.Errorf("из «%s» нет пути в колонку «%s»", st.Name, cache.Columns[tc].Name)
		}
		if err := c.transition(key, t.ID); err != nil {
			return "", fmt.Errorf("шаг «%s»: %w", t.Name, err)
		}
		if st = t.To; cache.columnOf(st.ID) != tc {
			if ts, err = c.transitions(key); err != nil {
				return "", err
			}
		}
	}
	return st.Name, nil
}

// findTransition matches by transition name, target status name or id:
// exact match first, then a unique prefix.
func findTransition(ts []Transition, q string) *Transition {
	q = strings.ToLower(strings.TrimSpace(q))
	var prefix []*Transition
	for i := range ts {
		name, to := strings.ToLower(ts[i].Name), strings.ToLower(ts[i].To.Name)
		if name == q || to == q || ts[i].ID == q {
			return &ts[i]
		}
		if strings.HasPrefix(name, q) || strings.HasPrefix(to, q) {
			prefix = append(prefix, &ts[i])
		}
	}
	if len(prefix) == 1 {
		return prefix[0]
	}
	return nil
}

// nextStep picks a transition into the target column (preferring the status
// named like it) or else into the column between cur and target closest to target.
func nextStep(ts []Transition, cache *Cache, cur, tc int) *Transition {
	var best *Transition
	bestD := len(cache.Columns)*2 + 2
	for i := range ts {
		col := cache.columnOf(ts[i].To.ID)
		if col < 0 || col != tc && cur >= 0 && (col-cur)*(tc-col) <= 0 {
			continue
		}
		d := 2 * max(tc-col, col-tc)
		if col == tc && !strings.EqualFold(ts[i].To.Name, cache.Columns[tc].Name) {
			d = 1
		}
		if d < bestD {
			best, bestD = &ts[i], d
		}
	}
	return best
}

// shortDate formats a Jira timestamp in local time.
func shortDate(s string) string {
	if t, err := parseTime(s); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return s
}

func parseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05.000-0700", s)
}
