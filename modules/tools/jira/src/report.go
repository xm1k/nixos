package main

// `jira report`: what I did with issues over a period, from their history.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type Event struct {
	Time                         time.Time
	Kind                         string // created | change | comment | worklog
	Field, From, To, Text, Spent string
	Seconds                      int
}

type Activity struct {
	Key, Summary, Type, Status string
	Events                     []Event
}

func reportCmd() *cobra.Command {
	var days int
	var week, lastWeek, short bool
	cmd := &cobra.Command{
		Use:     "report",
		Aliases: []string{"r", "week"},
		Short:   "Мои действия с задачами за период (по истории Jira) — для отчёта",
		Example: "  jira report              # последние 7 дней\n  jira report --last-week  # прошлая неделя пн–вс\n  jira report --week -s    # эта неделя, кратко",
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			since, until, err := reportPeriod(time.Now(), days, week, lastWeek)
			if err != nil {
				return err
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, dim("собираю историю из Jira…"))
			acts, err := c.activity(since, until)
			if err != nil {
				return err
			}
			writeReport(os.Stdout, c.me.String(), since, until, acts, short)
			return nil
		},
	}
	cmd.Flags().IntVarP(&days, "days", "n", 7, "за сколько последних дней (включая сегодня)")
	cmd.Flags().BoolVar(&week, "week", false, "текущая неделя с понедельника")
	cmd.Flags().BoolVar(&lastWeek, "last-week", false, "прошлая неделя пн–вс")
	cmd.Flags().BoolVarP(&short, "short", "s", false, "только задачи и что с ними стало")
	return cmd
}

func reportPeriod(now time.Time, days int, week, lastWeek bool) (since, until time.Time, err error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	switch {
	case lastWeek:
		return monday.AddDate(0, 0, -7), monday, nil
	case week:
		return monday, now, nil
	case days < 1:
		return since, until, errors.New("--days должно быть больше 0")
	}
	return today.AddDate(0, 0, 1-days), now, nil
}

// activity collects what I did in [since, until): created issues, field
// changes, comments, work logs. Jira cannot search "changed by me" for any
// field, so the net is wide and the history is filtered here.
func (c *Client) activity(since, until time.Time) ([]Activity, error) {
	me, err := c.myself()
	if err != nil {
		return nil, err
	}
	// JQL dates are in the server's zone: take a day of margin.
	from := strconv.Quote(since.Add(-24 * time.Hour).Format("2006-01-02 15:04"))
	jql := "updated >= " + from + " AND (assignee was currentUser() OR reporter = currentUser()" +
		" OR creator = currentUser() OR watcher = currentUser() OR issuekey in issueHistory()" +
		" OR status changed BY currentUser() AFTER " + from + ") ORDER BY updated DESC"
	fields := []string{"summary", "status", "issuetype", "created", "creator", "reporter", "comment", "worklog"}
	issues, err := c.search(jql, fields, "changelog", 1000)
	if err != nil {
		return nil, err
	}
	in := func(t time.Time) bool { return !t.Before(since) && t.Before(until) }
	var out []Activity
	for i := range issues {
		is := &issues[i]
		if err := c.complete(is); err != nil {
			return nil, err
		}
		f := &is.Fields
		a := Activity{Key: is.Key, Summary: f.Summary, Type: f.IssueType.String(), Status: is.status().Name}
		add := func(stamp string, author *User, e Event) {
			if t, err := parseTime(stamp); err == nil && in(t) && me.Is(author) {
				e.Time = t
				a.Events = append(a.Events, e)
			}
		}
		creator := f.Creator
		if creator == nil {
			creator = f.Reporter
		}
		add(f.Created, creator, Event{Kind: "created"})
		if is.Changelog != nil {
			for _, h := range is.Changelog.Histories {
				for _, it := range h.Items {
					switch field := strings.ToLower(it.Field); field {
					case "rank", "timespent", "timeestimate", "worklogid", "workratio":
					case "timeoriginalestimate":
						add(h.Created, h.Author, Event{Kind: "change", Field: it.Field, From: seconds(it.From), To: seconds(it.To)})
					default:
						add(h.Created, h.Author, Event{Kind: "change", Field: it.Field, From: it.FromString, To: it.ToString})
					}
				}
			}
		}
		if f.Comment != nil {
			for _, cm := range f.Comment.Comments {
				add(cm.Created, cm.Author, Event{Kind: "comment", Text: cm.Body})
			}
		}
		if f.Worklog != nil {
			for _, w := range f.Worklog.Worklogs {
				add(w.Started, w.Author, Event{Kind: "worklog", Spent: w.TimeSpent, Seconds: w.TimeSpentSeconds, Text: w.Comment})
			}
		}
		if len(a.Events) > 0 {
			sort.SliceStable(a.Events, func(i, j int) bool { return a.Events[i].Time.Before(a.Events[j].Time) })
			out = append(out, a)
		}
	}
	last := func(a Activity) time.Time { return a.Events[len(a.Events)-1].Time }
	sort.SliceStable(out, func(i, j int) bool { return last(out[i]).After(last(out[j])) })
	return out, nil
}

// complete refetches the history, comments and work logs that the search truncated.
func (c *Client) complete(is *Issue) error {
	f := &is.Fields
	if cl := is.Changelog; cl != nil && cl.Total > len(cl.Histories) || f.Comment != nil && f.Comment.Total > len(f.Comment.Comments) {
		var full Issue
		if err := c.do("GET", issuePath(is.Key)+"?expand=changelog&fields=comment", nil, &full); err != nil {
			return err
		}
		is.Changelog, f.Comment = full.Changelog, full.Fields.Comment
	}
	if f.Worklog != nil && f.Worklog.Total > len(f.Worklog.Worklogs) {
		return c.do("GET", issuePath(is.Key)+"/worklog", nil, f.Worklog)
	}
	return nil
}

func seconds(s string) string {
	if n, _ := strconv.Atoi(s); n > 0 {
		return formatDuration(n)
	}
	return ""
}

// formatDuration renders seconds like Jira: "1d 2h 30m" with 8h days.
func formatDuration(sec int) string {
	m := sec / 60
	var parts []string
	for _, u := range []struct {
		n    int
		unit string
	}{{m / 480, "d"}, {m / 60 % 8, "h"}, {m % 60, "m"}} {
		if u.n > 0 {
			parts = append(parts, strconv.Itoa(u.n)+u.unit)
		}
	}
	if len(parts) == 0 {
		return "0m"
	}
	return strings.Join(parts, " ")
}

var fieldNames = map[string]string{
	"status": "статус", "assignee": "исполнитель", "resolution": "резолюция", "summary": "заголовок",
	"description": "описание", "labels": "метки", "sprint": "спринт", "link": "связь", "priority": "приоритет",
	"epic link": "эпик", "parent": "родитель", "issuetype": "тип", "timeoriginalestimate": "оценка",
	"attachment": "вложение", "reporter": "автор", "duedate": "срок", "environment": "окружение",
}

func eventText(e Event) string {
	switch e.Kind {
	case "created":
		return "создана"
	case "comment":
		return "комментарий: «" + clip(e.Text, 160) + "»"
	case "worklog":
		return strings.TrimSuffix("списано "+e.Spent+": "+clip(e.Text, 120), ": ")
	}
	field := strings.ToLower(e.Field)
	name := e.Field
	if n, ok := fieldNames[field]; ok {
		name = n
	}
	from, to := clip(e.From, 80), clip(e.To, 80)
	switch {
	case field == "description" || field == "environment" || from == "" && to == "":
		return name + " изменено"
	case from == "":
		return name + ": " + to
	case to == "":
		return name + ": убрано «" + from + "»"
	}
	return name + ": " + from + " → " + to
}

// statusPath summarizes status changes: "To Do → In Progress → Review".
func statusPath(a Activity) string {
	var path []string
	for _, e := range a.Events {
		if e.Kind == "change" && strings.EqualFold(e.Field, "status") {
			if len(path) == 0 {
				path = append(path, e.From)
			}
			path = append(path, e.To)
		}
	}
	return strings.Join(path, " → ")
}

var weekdays = []string{"Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"}

func writeReport(w io.Writer, user string, since, until time.Time, acts []Activity, short bool) {
	fmt.Fprintf(w, "# Активность %s — %s · %s\n\n", since.Format("02.01"), until.Add(-time.Second).Format("02.01.2006"), user)
	if len(acts) == 0 {
		fmt.Fprintln(w, "Ничего не найдено.")
		return
	}
	var created, moves, comments, spent int
	for _, a := range acts {
		for _, e := range a.Events {
			switch {
			case e.Kind == "created":
				created++
			case e.Kind == "comment":
				comments++
			case e.Kind == "worklog":
				spent += e.Seconds
			case strings.EqualFold(e.Field, "status"):
				moves++
			}
		}
	}
	fmt.Fprintf(w, "Задач: %d · создано: %d · переходов: %d · комментариев: %d", len(acts), created, moves, comments)
	if spent > 0 {
		fmt.Fprint(w, " · списано: "+formatDuration(spent))
	}
	fmt.Fprintln(w)
	if short {
		fmt.Fprintln(w)
	}
	for _, a := range acts {
		if short {
			what := statusPath(a)
			if what == "" {
				what = a.Status
			}
			if a.Events[0].Kind == "created" {
				what = "создана; " + what
			}
			fmt.Fprintf(w, "- %s %s — %s\n", a.Key, a.Summary, what)
			continue
		}
		fmt.Fprintf(w, "\n## %s %s\n*%s · сейчас: %s*\n\n", a.Key, a.Summary, a.Type, a.Status)
		for _, e := range a.Events {
			t := e.Time.Local()
			fmt.Fprintf(w, "- %s %s — %s\n", weekdays[t.Weekday()], t.Format("02.01 15:04"), eventText(e))
		}
	}
}
