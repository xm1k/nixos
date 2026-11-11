package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func table() *tabwriter.Writer { return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0) }

func idLabel(id int) string {
	if id <= 0 {
		return ""
	}
	return bold(strconv.Itoa(id))
}

func ago(t time.Time) string {
	switch d := time.Since(t); {
	case d < time.Minute:
		return "только что"
	case d < time.Hour:
		return fmt.Sprintf("%d мин назад", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d ч назад", int(d.Hours()))
	}
	return t.Format("02.01 15:04")
}

// syncFirst refreshes the cache in the foreground when -s is given.
func syncFirst(sync bool) error {
	if sync {
		return runNow(&Op{Kind: "sync"})
	}
	return nil
}

// ---- list / current / epics -----------------------------------------------------

func listCmd() *cobra.Command {
	var all, sync bool
	cmd := &cobra.Command{
		Use:       "list [e|epic|t|ticket]",
		Aliases:   []string{"ls", "l"},
		Short:     "Мои задачи из кэша и очередь (e — только эпики, t — только тикеты)",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"epic", "ticket"},
		RunE: func(_ *cobra.Command, args []string) error {
			kind := ""
			if len(args) == 1 {
				kind = map[string]string{"e": "epic", "epic": "epic", "t": "ticket", "ticket": "ticket"}[strings.ToLower(args[0])]
				if kind == "" {
					return fmt.Errorf("неизвестный фильтр %q: e|epic или t|ticket", args[0])
				}
			}
			if err := syncFirst(sync); err != nil {
				return err
			}
			return printList(all, kind)
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "включая закрытые за 2 недели и Verified")
	cmd.Flags().BoolVarP(&sync, "sync", "s", false, "сначала обновить кэш (ждать Jira)")
	return cmd
}

// printList shows cached issues with pending moves on top; kind "epic" or
// "ticket" keeps only epics or only tickets.
func printList(all bool, kind string) error {
	cache, q := loadCache(), loadQueue()
	keep := func(epic bool) bool { return kind == "" || (kind == "epic") == epic }
	moving, syncQueued := map[string]string{}, false
	for _, op := range q.Pending {
		switch op.Kind {
		case "create":
			if keep(strings.EqualFold(op.Issue.Type, "Epic")) {
				fmt.Printf("%s %s  %s\n", bold("@"+strconv.Itoa(op.ID)), yellow("⏳ создаётся"), op.Issue.Summary)
			}
		case "move":
			moving[op.Key] = op.Target
		case "sync":
			syncQueued = true
		}
	}
	issues := append([]Item(nil), cache.Issues...)
	if kind == "epic" { // my epics that are not on the board too
		for _, e := range cache.Epics {
			if !containsKey(cache.Issues, e.Key) {
				issues = append(issues, e)
			}
		}
	}
	sort.SliceStable(issues, func(i, j int) bool {
		return cache.columnOf(issues[i].StatusID) < cache.columnOf(issues[j].StatusID)
	})
	w := table()
	for _, is := range issues {
		to, isMoving := moving[is.Key]
		if !keep(cache.isEpic(is)) || !all && hidden(is) && !isMoving {
			continue
		}
		status := statusPaint(is.Status, is.Category)
		if isMoving {
			status = dim(is.Status+" →") + " " + yellow(to+" ⏳")
			delete(moving, is.Key)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", idLabel(is.ID), dim(is.Key), status, is.Summary)
	}
	for key, to := range moving { // issues not in the cache (yet)
		if kind == "" {
			fmt.Fprintf(w, "\t%s\t%s\t\n", dim(key), yellow("→ "+to+" ⏳"))
		}
	}
	w.Flush()
	if len(q.Failed) > 0 {
		fmt.Printf("%s ошибок в очереди: %d — %s\n", red("✗"), len(q.Failed), dim("jira q"))
	}
	stale := time.Since(cache.Updated) > 5*time.Minute
	switch {
	case cache.Updated.IsZero():
		fmt.Println(dim("кэш пуст — загружаю в фоне, повторите через несколько секунд"))
	case stale:
		fmt.Println(dim("обновлено " + ago(cache.Updated) + ", обновляю в фоне"))
	}
	if stale && !syncQueued {
		return enqueue(&Op{Kind: "sync"})
	}
	return nil
}

func containsKey(items []Item, key string) bool {
	for _, is := range items {
		if is.Key == key {
			return true
		}
	}
	return false
}

// inProgress returns cached tickets (not epics) in the "In Progress" status.
func inProgress(cache *Cache) []Item {
	var out []Item
	for _, is := range cache.Issues {
		if strings.EqualFold(is.Status, "In Progress") && !cache.isEpic(is) {
			out = append(out, is)
		}
	}
	return out
}

func currentCmd() *cobra.Command {
	var sync, verbose bool
	cmd := &cobra.Command{
		Use:     "current",
		Aliases: []string{"cur"},
		Short:   "Ключ тикета (не эпика) в In Progress, напр. RE-123",
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := syncFirst(sync); err != nil {
				return err
			}
			issues := inProgress(loadCache())
			if len(issues) == 0 {
				fmt.Fprintln(os.Stderr, dim("нет задач в In Progress"))
			}
			for i, is := range issues {
				if !verbose {
					fmt.Println(is.Key)
				} else if err := showIssue(is.Key, 0, i > 0); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&sync, "sync", "s", false, "сначала обновить кэш (ждать Jira)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "подробно, из Jira")
	return cmd
}

func epicsCmd() *cobra.Command {
	var sync bool
	cmd := &cobra.Command{
		Use:   "epics",
		Short: "Мои открытые эпики (из кэша)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := syncFirst(sync); err != nil {
				return err
			}
			epics := loadCache().Epics
			if len(epics) == 0 {
				fmt.Println(dim("на вас нет открытых эпиков (jira epics -s — обновить)"))
			}
			for _, e := range epics {
				fmt.Printf("%s  %s  %s\n", bold(fmt.Sprintf("%3d", e.ID)), dim(e.Key), e.Summary)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&sync, "sync", "s", false, "сначала обновить кэш (ждать Jira)")
	return cmd
}

// ---- pickers --------------------------------------------------------------------

// issueArg takes the issue from args[0] or lets me pick one.
func issueArg(args []string, header string) (string, error) {
	if len(args) > 0 {
		return issueKey(args[0]), nil
	}
	if !interactive() {
		return "", errors.New("укажите задачу: номер из jira list или ключ")
	}
	var options, keys []string
	for _, op := range loadQueue().Pending {
		if op.Kind == "create" {
			ref := "@" + strconv.Itoa(op.ID)
			options, keys = append(options, ref+"  "+op.Issue.Summary+"  (создаётся)"), append(keys, ref)
		}
	}
	cache := loadCache()
	issues := append([]Item(nil), cache.Issues...)
	sort.SliceStable(issues, func(i, j int) bool {
		return cache.columnOf(issues[i].StatusID) < cache.columnOf(issues[j].StatusID)
	})
	for _, is := range issues {
		if !hidden(is) {
			options = append(options, fmt.Sprintf("%d  %s  [%s]  %s", is.ID, is.Key, is.Status, is.Summary))
			keys = append(keys, is.Key)
		}
	}
	i, err := choose(header, options)
	if err != nil {
		return "", err
	}
	return keys[i], nil
}

func askEstimate() (string, error) {
	for {
		s, err := ask("Original estimate", "30m, 2h, 1d 4h — Enter без оценки")
		if err != nil || validEstimate(s) == nil {
			return s, err
		}
		fmt.Fprintln(os.Stderr, red(validEstimate(s).Error()))
	}
}

// askEpic picks one of my epics; special options go first, after the last used epic.
func askEpic(special ...[2]string) (string, error) {
	epics, last := loadCache().Epics, lastEpic()
	if len(epics) == 0 {
		fmt.Fprintln(os.Stderr, dim("на вас нет открытых эпиков (jira epics -s) — без эпика"))
		return "", nil
	}
	var options, values []string
	for _, e := range epics {
		if e.Key == last {
			options, values = append(options, fmt.Sprintf("%d  %s  %s  (последний)", e.ID, e.Key, e.Summary)), append(values, e.Key)
		}
	}
	for _, sp := range special {
		options, values = append(options, sp[0]), append(values, sp[1])
	}
	for _, e := range epics {
		if e.Key != last {
			options, values = append(options, fmt.Sprintf("%d  %s  %s", e.ID, e.Key, e.Summary)), append(values, e.Key)
		}
	}
	i, err := choose("Эпик", options)
	if err != nil {
		return "", err
	}
	if v := values[i]; v != "" && v != noEpic {
		setLastEpic(v)
	}
	return values[i], nil
}

// ---- create / move / set / comment ----------------------------------------------

func createCmd() *cobra.Command {
	var n NewIssue
	var wait, noAsk, doSwitch bool
	cmd := &cobra.Command{
		Use:     "create [заголовок] [описание...]",
		Aliases: []string{"c", "new"},
		Short:   "Создать задачу на себя (в фоне); спросит оценку и эпик",
		Example: `  jira create "Починить логин" "После деплоя 500 на /login"
  jira create "Починить логин" -E 4h --epic 12
  jira create "Починить логин" --switch   # и сразу в In Progress
  git log -1 --format=%B | jira create "Из коммита" -d -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if n.Project = strings.ToUpper(cfg.Project); n.Project == "" {
				return errors.New("не задан default_project: jira init")
			}
			if len(args) > 0 {
				n.Summary = args[0]
				n.Description = strings.Join(append([]string{n.Description}, args[1:]...), " ")
			}
			if n.Description, err = textArg(n.Description); err != nil {
				return err
			}
			if n.Summary == "" && interactive() {
				if n.Summary, err = ask("Заголовок", "что сделать"); err == nil && n.Description == "" {
					n.Description, err = askText("Описание (можно пустое)")
				}
				if err != nil {
					return err
				}
			}
			if n.Summary = strings.TrimSpace(n.Summary); n.Summary == "" {
				return errCancelled
			}
			if err := validEstimate(n.Estimate); err != nil {
				return err
			}
			n.Epic = issueKey(n.Epic)
			if !noAsk && interactive() {
				if !cmd.Flags().Changed("estimate") {
					if n.Estimate, err = askEstimate(); err != nil {
						return err
					}
				}
				if !cmd.Flags().Changed("epic") {
					if n.Epic, err = askEpic([2]string{"— без эпика", ""}); err != nil {
						return err
					}
				}
			}
			op := &Op{Kind: "create", Issue: &n}
			if err := submit(op, wait); err != nil || !doSwitch {
				return err
			}
			if !wait { // queued: the move goes after the create and finds the key by @id
				op.Key = "@" + strconv.Itoa(op.ID)
			}
			return switchTo(op.Key, wait)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&n.Description, "description", "d", "", "описание ('-' — из stdin)")
	f.StringVarP(&n.Type, "type", "t", "", "тип задачи (по умолчанию Task)")
	f.StringSliceVarP(&n.Labels, "label", "l", nil, "метки")
	f.StringVarP(&n.Estimate, "estimate", "E", "", "original estimate: 30m, 2h, 1d 4h")
	f.StringVar(&n.Epic, "epic", "", "эпик: номер или ключ")
	f.BoolVarP(&wait, "wait", "w", false, "дождаться ответа Jira, без очереди")
	f.BoolVarP(&noAsk, "no-ask", "y", false, "не спрашивать оценку и эпик")
	f.BoolVarP(&doSwitch, "switch", "S", false, "сразу взять в работу (jira switch)")
	_ = cmd.RegisterFlagCompletionFunc("epic", completeEpics)
	return cmd
}

func moveCmd() *cobra.Command {
	var wait bool
	var comment string
	cmd := &cobra.Command{
		Use:     "move [задача] [колонка|статус|номер колонки]",
		Aliases: []string{"mv", "m"},
		Short:   "Перенести задачу (в фоне); без аргументов — выбор задачи и колонки",
		Example: `  jira move                  # выбрать задачу, затем колонку
  jira move 3 in progress    # по имени колонки, статуса или перехода
  jira move 3 2              # во вторую колонку доски
  jira move 3 done -c "Выкатили"   # в закрывающую колонку — с комментарием`,
		ValidArgsFunction: completeMove,
		RunE: func(_ *cobra.Command, args []string) error {
			key, err := issueArg(args, "Какую задачу перенести?")
			if err != nil {
				return err
			}
			cache := loadCache()
			target := strings.Join(args[min(1, len(args)):], " ")
			if target == "" {
				if !interactive() || len(cache.Columns) == 0 {
					return errors.New("укажите колонку: jira move <задача> <колонка>")
				}
				header, cur := "Куда?", -1
				if is := cache.find(key); is != nil {
					header, cur = key+" ["+is.Status+"] — куда?", cache.columnOf(is.StatusID)
				}
				var options []string
				for i, col := range cache.Columns {
					label := fmt.Sprintf("%d  %s", i+1, col.Name)
					if i == cur {
						label += "  (сейчас здесь)"
					}
					options = append(options, label)
				}
				i, err := choose(header, options)
				if err != nil {
					return err
				}
				target = cache.Columns[i].Name
			}
			if i, err := strconv.Atoi(target); err == nil && i >= 1 && i <= len(cache.Columns) {
				target = cache.Columns[i-1].Name
			}
			if comment, err = moveComment(cache, key, target, comment); err != nil {
				return err
			}
			return submit(&Op{Kind: "move", Key: key, Target: target, Comment: comment}, wait)
		},
	}
	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "дождаться ответа Jira, без очереди")
	cmd.Flags().StringVarP(&comment, "comment", "c", "", "комментарий после переноса ('-' — из stdin)")
	return cmd
}

// moveComment reads the -c text ('-' = stdin); a move into a closing column
// requires one, asked for when interactive.
func moveComment(cache *Cache, key, target, comment string) (string, error) {
	comment, err := textArg(comment)
	if err != nil || comment != "" || !cache.closing(target) {
		return comment, err
	}
	if !interactive() {
		return "", fmt.Errorf("перенос в «%s» требует комментария: -c \"что сделано\"", target)
	}
	if comment, err = askText(key + " → " + target + ": комментарий обязателен (что сделано)"); err != nil {
		return "", err
	}
	if comment == "" {
		return "", errors.New("без комментария перенос в «" + target + "» отменён")
	}
	return comment, nil
}

// Board columns used by switch and done.
const (
	pendingColumn  = "Pending"
	progressColumn = "In Progress"
	doneColumn     = "Done"
)

// switchTo puts key into In Progress; whatever else is there goes to Pending.
func switchTo(key string, wait bool) error {
	cur := inProgress(loadCache())
	for _, is := range cur {
		if is.Key != key {
			if err := submit(&Op{Kind: "move", Key: is.Key, Target: pendingColumn}, wait); err != nil {
				return err
			}
		}
	}
	if containsKey(cur, key) {
		fmt.Println(dim(key + " уже в " + progressColumn))
		return nil
	}
	return submit(&Op{Kind: "move", Key: key, Target: progressColumn}, wait)
}

func switchCmd() *cobra.Command {
	var wait bool
	cmd := &cobra.Command{
		Use:     "switch [задача]",
		Aliases: []string{"sw"},
		Short:   "Взять задачу в работу (In Progress), текущую — в Pending (в фоне)",
		Example: `  jira switch          # выбрать задачу
  jira switch 3
  jira switch 48268    # RE-48268`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeIssues,
		RunE: func(_ *cobra.Command, args []string) error {
			key, err := issueArg(args, "Какую задачу взять в работу?")
			if err != nil {
				return err
			}
			return switchTo(key, wait)
		},
	}
	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "дождаться ответа Jira, без очереди")
	return cmd
}

// currentKey returns the issue in In Progress, asking when there are several.
func currentKey(header string) (string, error) {
	cur := inProgress(loadCache())
	switch {
	case len(cur) == 0:
		return "", errors.New("нет задач в " + progressColumn + ": укажите задачу")
	case len(cur) == 1:
		return cur[0].Key, nil
	case !interactive():
		return "", errors.New("в " + progressColumn + " несколько задач: укажите задачу")
	}
	options := make([]string, len(cur))
	for i, is := range cur {
		options[i] = fmt.Sprintf("%d  %s  %s", is.ID, is.Key, is.Summary)
	}
	i, err := choose(header, options)
	if err != nil {
		return "", err
	}
	return cur[i].Key, nil
}

func doneCmd() *cobra.Command {
	var wait bool
	var comment string
	cmd := &cobra.Command{
		Use:   "done [задача]",
		Short: "Перенести задачу (по умолчанию текущую из In Progress) в Done с комментарием (в фоне)",
		Example: `  jira done -c "Выкатили"     # текущую задачу (jira cur)
  jira done                   # спросит комментарий
  jira done 3 -c -            # комментарий из stdin`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeIssues,
		RunE: func(_ *cobra.Command, args []string) error {
			var key string
			var err error
			if len(args) > 0 {
				key = issueKey(args[0])
			} else if key, err = currentKey("Какую задачу закрыть?"); err != nil {
				return err
			}
			if comment, err = moveComment(loadCache(), key, doneColumn, comment); err != nil {
				return err
			}
			return submit(&Op{Kind: "move", Key: key, Target: doneColumn, Comment: comment}, wait)
		},
	}
	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "дождаться ответа Jira, без очереди")
	cmd.Flags().StringVarP(&comment, "comment", "c", "", "комментарий: что сделано ('-' — из stdin)")
	return cmd
}

func setCmd() *cobra.Command {
	var op = Op{Kind: "set"}
	var wait bool
	cmd := &cobra.Command{
		Use:               "set [задача]",
		Short:             "Задать оценку и эпик задаче (в фоне); без флагов — спросит",
		Example:           "  jira set 3 -E 4h --epic 12\n  jira set 3 --epic none   # убрать из эпика",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeIssues,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if op.Key, err = issueArg(args, "Какой задаче задать оценку/эпик?"); err != nil {
				return err
			}
			if err := validEstimate(op.Estimate); err != nil {
				return err
			}
			if op.Epic != noEpic {
				op.Epic = issueKey(op.Epic)
			}
			if !cmd.Flags().Changed("estimate") && !cmd.Flags().Changed("epic") {
				if !interactive() {
					return errors.New("укажите -E и/или --epic")
				}
				if op.Estimate, err = askEstimate(); err != nil {
					return err
				}
				if op.Epic, err = askEpic([2]string{"— не менять", ""}, [2]string{"— убрать из эпика", noEpic}); err != nil {
					return err
				}
			}
			if op.Estimate == "" && op.Epic == "" {
				fmt.Println(dim("нечего менять"))
				return nil
			}
			return submit(&op, wait)
		},
	}
	cmd.Flags().StringVarP(&op.Estimate, "estimate", "E", "", "original estimate: 30m, 2h, 1d 4h")
	cmd.Flags().StringVar(&op.Epic, "epic", "", "эпик: номер или ключ, none — убрать")
	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "дождаться ответа Jira, без очереди")
	_ = cmd.RegisterFlagCompletionFunc("epic", completeEpics)
	return cmd
}

func commentCmd() *cobra.Command {
	var wait bool
	cmd := &cobra.Command{
		Use:               "comment [задача] [текст...]",
		Aliases:           []string{"cm"},
		Short:             "Добавить комментарий (в фоне); текст из аргументов, stdin ('-') или gum",
		ValidArgsFunction: completeIssues,
		RunE: func(_ *cobra.Command, args []string) error {
			key, err := issueArg(args, "Какую задачу прокомментировать?")
			if err != nil {
				return err
			}
			text, err := textArg(args[min(1, len(args)):]...)
			if err == nil && text == "" {
				if !interactive() {
					return errors.New("нет текста: jira comment <задача> \"текст\"")
				}
				text, err = askText("Комментарий к " + key)
			}
			if err != nil {
				return err
			}
			if text == "" {
				return errCancelled
			}
			return submit(&Op{Kind: "comment", Key: key, Comment: text}, wait)
		},
	}
	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "дождаться ответа Jira, без очереди")
	return cmd
}

// ---- view -----------------------------------------------------------------------

func viewCmd() *cobra.Command {
	var comments int
	var web bool
	cmd := &cobra.Command{
		Use:               "view [задача]",
		Aliases:           []string{"v", "show", "open", "o"},
		Short:             "Показать задачу из Jira: поля, описание, комментарии (--web — в браузере)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeIssues,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := issueArg(args, "Какую задачу показать?")
			if err == nil {
				key, err = resolveRef(key)
			}
			if err != nil {
				return err
			}
			if web || cmd.CalledAs() == "open" || cmd.CalledAs() == "o" {
				cfg, _ := loadConfig()
				return exec.Command("xdg-open", cfg.URL+"/browse/"+key).Start()
			}
			return showIssue(key, comments, false)
		},
	}
	cmd.Flags().IntVarP(&comments, "comments", "n", 5, "сколько последних комментариев (-1 — все)")
	cmd.Flags().BoolVar(&web, "web", false, "открыть в браузере (= jira open)")
	return cmd
}

func showIssue(key string, comments int, gap bool) error {
	c, err := newClient()
	if err != nil {
		return err
	}
	is, err := c.issue(key, "summary", "description", "status", "assignee", "reporter", "priority",
		"issuetype", "labels", "created", "updated", "comment")
	if err != nil {
		return err
	}
	if gap {
		fmt.Println()
	}
	f := &is.Fields
	fmt.Println(bold(is.Key + "  " + f.Summary))
	st := is.status()
	for _, r := range [][2]string{
		{"Статус", statusPaint(st.Name, st.StatusCategory.Key)}, {"Тип", f.IssueType.String()},
		{"Приоритет", f.Priority.String()}, {"Исполнитель", f.Assignee.String()}, {"Автор", f.Reporter.String()},
		{"Метки", strings.Join(f.Labels, ", ")}, {"Создана", shortDate(f.Created)},
		{"Обновлена", shortDate(f.Updated)}, {"Ссылка", dim(c.URL + "/browse/" + is.Key)},
	} {
		if r[1] != "" {
			fmt.Printf("  %-12s %s\n", dim(r[0]), r[1])
		}
	}
	if f.Description != "" {
		fmt.Println("\n" + indent(f.Description))
	}
	if f.Comment == nil || len(f.Comment.Comments) == 0 || comments == 0 {
		return nil
	}
	list := f.Comment.Comments
	if comments > 0 && len(list) > comments {
		list = list[len(list)-comments:]
	}
	fmt.Println(dim(fmt.Sprintf("\nКомментарии: %d из %d", len(list), max(f.Comment.Total, len(list)))))
	for _, cm := range list {
		fmt.Printf("\n  %s  %s\n%s\n", bold(cm.Author.String()), dim(shortDate(cm.Created)), indent(cm.Body))
	}
	return nil
}

func indent(s string) string { return "  " + strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n  ") }

// ---- queue / sync / init --------------------------------------------------------

func queueCmd() *cobra.Command {
	var last int
	cmd := &cobra.Command{
		Use:     "queue",
		Aliases: []string{"q"},
		Short:   "Очередь: что ждёт отправки, что упало, что сделано",
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			q, done := loadQueue(), doneLog()
			w := table()
			for _, d := range done[max(0, len(done)-last):] {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", green("✓"), dim("@"+strconv.Itoa(d.ID)), d.What, bold(d.Result), dim(ago(d.Time)))
				if d.Warn != "" {
					fmt.Fprintf(w, "\t\t%s\t\t\n", yellow("⚠ "+d.Warn))
				}
			}
			for _, op := range q.Failed {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t\n", red("✗"), dim("@"+strconv.Itoa(op.ID)), op, red(op.Error))
			}
			for _, op := range q.Pending {
				if op.Kind != "sync" {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t\n", yellow("⏳"), dim("@"+strconv.Itoa(op.ID)), op, dim(op.Error))
				}
			}
			w.Flush()
			if len(q.Failed) > 0 {
				fmt.Println(dim("jira q retry [id…] — повторить, jira q drop [id…] — убрать (без id — все упавшие)"))
			}
			if l, err := lock("worker.lock", false); err == nil && len(q.Pending) > 0 {
				l.Close()
				fmt.Println(dim("обработчик не запущен (Jira была недоступна?) — jira q run"))
			} else if err == nil {
				l.Close()
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&last, "last", "n", 15, "сколько последних выполненных показать")
	// moveFailed takes failed ops (all, or by id) out of Failed, back to Pending if retry.
	moveFailed := func(args []string, retry bool) (n int, err error) {
		ids := map[int]bool{}
		for _, a := range args {
			id, err := strconv.Atoi(strings.TrimPrefix(a, "@"))
			if err != nil {
				return 0, fmt.Errorf("неверный id %q", a)
			}
			ids[id] = true
		}
		return n, updateQueue(func(q *Queue) {
			var keep []*Op
			for _, op := range q.Failed {
				if len(ids) > 0 && !ids[op.ID] {
					keep = append(keep, op)
					continue
				}
				if n++; retry {
					op.Error = ""
					q.Pending = append(q.Pending, op)
				}
			}
			q.Failed = keep
		})
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Запустить обработку очереди",
		RunE:  func(*cobra.Command, []string) error { return spawnWorker() },
	}, &cobra.Command{
		Use:   "retry [id…]",
		Short: "Вернуть упавшие операции в очередь (все, если id не указаны)",
		RunE: func(_ *cobra.Command, args []string) error {
			n, err := moveFailed(args, true)
			if err != nil {
				return err
			}
			fmt.Println("в очередь возвращено:", n)
			return spawnWorker()
		},
	}, &cobra.Command{
		Use:   "drop [id…]",
		Short: "Убрать упавшие операции (все, если id не указаны)",
		RunE: func(_ *cobra.Command, args []string) error {
			n, err := moveFailed(args, false)
			fmt.Println("убрано:", n)
			return err
		},
	})
	return cmd
}

func syncCmd() *cobra.Command {
	var wait bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Обновить кэш задач и доски (в фоне)",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return submit(&Op{Kind: "sync"}, wait) },
	}
	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "дождаться")
	return cmd
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Настроить подключение к Jira",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			for _, f := range []struct {
				title string
				value *string
				extra []string
			}{
				{"URL Jira", &cfg.URL, nil},
				{"Personal Access Token (Enter — оставить текущий)", &cfg.Token, []string{"--password"}},
				{"Проект по умолчанию, например RE", &cfg.Project, nil},
				{"Доска по умолчанию (id, необязательно)", &cfg.Board, nil},
			} {
				args := append([]string{"input", "--header", f.title}, f.extra...)
				if f.extra == nil {
					args = append(args, "--value", *f.value)
				}
				v, err := gum("", args...)
				if err != nil {
					return err
				}
				if v != "" || f.extra == nil {
					*f.value = v
				}
			}
			cfg.URL = strings.TrimRight(cfg.URL, "/")
			c := &Client{Config: cfg}
			me, err := c.myself()
			if err != nil {
				return fmt.Errorf("проверка не прошла, не сохранено: %w", err)
			}
			fmt.Println(green("✓ Подключено как " + me.String()))
			return writeJSON(configPath(), cfg)
		},
	}
}

// ---- completion (from the cache) ------------------------------------------------

func completions(items []Item) []string {
	var out []string
	for _, is := range items {
		out = append(out, fmt.Sprintf("%d\t%s · %s · %s", is.ID, is.Key, is.Status, is.Summary))
	}
	return out
}

func completeIssues(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var open []Item
	for _, is := range loadCache().Issues {
		if !hidden(is) {
			open = append(open, is)
		}
	}
	return completions(open), cobra.ShellCompDirectiveNoFileComp
}

func completeEpics(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return completions(loadCache().Epics), cobra.ShellCompDirectiveNoFileComp
}

func completeMove(cmd *cobra.Command, args []string, s string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 1 {
		return completeIssues(cmd, args, s)
	}
	var out []string
	for i, col := range loadCache().Columns {
		out = append(out, col.Name+"\tколонка "+strconv.Itoa(i+1))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
