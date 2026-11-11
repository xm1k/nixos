package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// ---------- styles ----------

var (
	cAccent = lipgloss.AdaptiveColor{Light: "#c2185b", Dark: "#ff4f9a"}
	cDim    = lipgloss.AdaptiveColor{Light: "#7a7a7a", Dark: "#7c7c8a"}
	cSel    = lipgloss.AdaptiveColor{Light: "#e8e0ef", Dark: "#2c2838"}
	cErr    = lipgloss.AdaptiveColor{Light: "#c62828", Dark: "#ff6b6b"}

	sAccent = lipgloss.NewStyle().Foreground(cAccent)
	sBold   = lipgloss.NewStyle().Bold(true)
	sDim    = lipgloss.NewStyle().Foreground(cDim)
	sErr    = lipgloss.NewStyle().Foreground(cErr)
	sHeader = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sTabOn  = lipgloss.NewStyle().Foreground(cAccent).Bold(true).Underline(true)
	sTabOff = lipgloss.NewStyle().Foreground(cDim)
	sSel    = lipgloss.NewStyle().Background(cSel)
)

// ---------- model ----------

const (
	kSong = iota
	kAlbum
	kArtist
	kPlaylist
	kMix // a playlist played as radio
	kMyAlbum
	kWaveMode
	kHeader
	kText
	kNewAlbum
)

type item struct {
	kind  int
	track *Track
	id    string
	num   int
	title string
	sub   string
	check *bool // album picker
}

func (it item) selectable() bool { return it.kind != kHeader && it.kind != kText }

type view struct {
	kind    string // home search likes history albums queue album artist playlist myalbum lyrics picker
	title   string
	id      string
	num     int
	target  *Track // picker
	items   []item
	cursor  int
	offset  int
	loading bool
	err     string
	loaded  bool
}

type promptState struct {
	label  string
	input  textinput.Model
	submit func(string) tea.Cmd
}

var tabNames = []string{"Волна", "Поиск", "Любимое", "История", "Альбомы", "Очередь"}

const tabQueue = 5

type model struct {
	api    *API
	w, h   int
	tab    int
	stacks [][]*view
	search textinput.Model
	typing bool
	filter int
	prompt *promptState
	st     *Status
	stErr  error
	qver   int
	liked  map[string]bool
	msg    string
	msgErr bool
	msgAt  time.Time
	help   bool
}

var searchFilters = []struct{ id, name string }{
	{"songs", "треки"}, {"albums", "альбомы"}, {"artists", "исполнители"}, {"playlists", "плейлисты"}, {"", "всё"},
}

func runTUI() error {
	if loadToken() == "" {
		fmt.Println("Сначала войдём в b.net.")
		if err := cmdLogin().Execute(); err != nil {
			return err
		}
	}
	c, err := dialDaemon(true)
	if err != nil {
		return err
	}
	c.Close()
	ti := textinput.New()
	ti.Placeholder = "что ищем?"
	ti.Prompt = "/ "
	ti.CharLimit = 200
	m := &model{api: NewAPI(), search: ti, liked: map[string]bool{}}
	m.stacks = make([][]*view, len(tabNames))
	m.stacks[0] = []*view{{kind: "home", title: "Моя волна"}}
	m.stacks[1] = []*view{{kind: "search", title: "Поиск", loaded: true}}
	m.stacks[2] = []*view{{kind: "likes", title: "Любимое"}}
	m.stacks[3] = []*view{{kind: "history", title: "История"}}
	m.stacks[4] = []*view{{kind: "albums", title: "Мои альбомы"}}
	m.stacks[5] = []*view{{kind: "queue", title: "Очередь"}}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// ---------- messages ----------

type tickMsg time.Time
type statusMsg struct {
	st  *Status
	err error
}
type loadedMsg struct {
	v     *view
	items []item
	err   error
}
type queueMsg struct {
	q   *queueResp
	err error
}
type likedMsg map[string]bool
type noteMsg struct {
	text string
	err  error
}

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchStatus() tea.Msg {
	s, err := getStatus()
	return statusMsg{s, err}
}

func fetchQueue() tea.Msg {
	q, err := getQueue()
	return queueMsg{q, err}
}

func fetchLiked() tea.Msg {
	var l map[string]bool
	if rpc("liked", nil, &l, false) != nil {
		return nil
	}
	return likedMsg(l)
}

// do runs a daemon command in the background and reports its error.
func do(cmd string, args any) tea.Cmd {
	return func() tea.Msg {
		if err := rpc(cmd, args, nil, true); err != nil {
			return noteMsg{err: err}
		}
		return fetchStatus()
	}
}

func note(s string) tea.Cmd { return func() tea.Msg { return noteMsg{text: s} } }

func (m *model) Init() tea.Cmd {
	return tea.Batch(tick(), fetchStatus, fetchLiked, m.load(m.cur()))
}

func (m *model) cur() *view {
	s := m.stacks[m.tab]
	return s[len(s)-1]
}

func (m *model) push(v *view) tea.Cmd {
	m.stacks[m.tab] = append(m.stacks[m.tab], v)
	return m.load(v)
}

func (m *model) pop() {
	if s := m.stacks[m.tab]; len(s) > 1 {
		m.stacks[m.tab] = s[:len(s)-1]
	}
}

func (m *model) setTab(i int) tea.Cmd {
	m.tab = i
	m.typing = false
	v := m.cur()
	if i == tabQueue {
		return fetchQueue
	}
	if !v.loaded && !v.loading {
		return m.load(v)
	}
	return nil
}

// ---------- loading views ----------

func trackItems(ts []Track) []item {
	out := make([]item, 0, len(ts))
	for i := range ts {
		t := ts[i]
		out = append(out, item{kind: kSong, track: &t, title: t.Title, sub: t.ArtistNames()})
	}
	return out
}

func resultItem(r SearchResult) (item, bool) {
	title, sub := resultText(r)
	it := item{title: title, sub: sub}
	b := r.BrowseID
	switch {
	case r.ResultType == "song" || r.ResultType == "video" || (r.VideoID != "" && b == "" && r.PlaylistID == ""):
		t := r.Track
		it.kind, it.track, it.sub = kSong, &t, t.ArtistNames()
	case strings.HasPrefix(b, "MPRE") || r.ResultType == "album":
		it.kind, it.id = kAlbum, b
		if it.sub == "" || r.ResultType == "" {
			it.sub = "альбом · " + r.ArtistNames()
		}
	case strings.HasPrefix(b, "UC") || r.ResultType == "artist":
		it.kind, it.id = kArtist, b
	case strings.HasPrefix(b, "VL") || r.ResultType == "playlist":
		it.kind, it.id = kPlaylist, b
		if r.ResultType == "" {
			it.sub = "плейлист"
		}
	case r.PlaylistID != "":
		it.kind, it.id, it.sub = kMix, r.PlaylistID, "микс"
	default:
		return it, false
	}
	return it, it.title != "" && (it.track != nil || it.id != "")
}

func header(s string) item { return item{kind: kHeader, title: s} }

func (m *model) load(v *view) tea.Cmd {
	if v.kind == "search" || v.kind == "queue" {
		return nil
	}
	v.loading, v.err = true, ""
	a := m.api
	filter := searchFilters[m.filter].id
	return func() tea.Msg {
		var items []item
		var err error
		switch v.kind {
		case "home":
			for _, md := range WaveModes {
				items = append(items, item{kind: kWaveMode, id: md.ID, title: "≋ " + md.Name, sub: md.Desc})
			}
			if t, e := a.Taste(); e == nil && t.Ready {
				var names []string
				for i, x := range t.Artists {
					if i < 5 {
						names = append(names, x.Name)
					}
				}
				items = append(items, item{kind: kText, title: fmt.Sprintf("любимых %d · наслушано %.1f ч · %s", t.Likes, t.Hours, strings.Join(names, ", "))})
			} else if e != nil {
				err = e
			}
			if shelves, e := a.Home(); e == nil {
				for _, sh := range shelves {
					var row []item
					for _, c := range sh.Contents {
						if it, ok := resultItem(c); ok {
							row = append(row, it)
						}
					}
					if len(row) > 0 {
						items = append(items, header(sh.Title))
						items = append(items, row...)
					}
				}
			} else if err == nil {
				err = e
			}
		case "search-results":
			var res []SearchResult
			res, err = a.Search(v.id, filter, 40)
			lastCat := ""
			for _, r := range res {
				if filter == "" && r.Category != lastCat && r.Category != "" {
					items = append(items, header(r.Category))
					lastCat = r.Category
				}
				if it, ok := resultItem(r); ok {
					items = append(items, it)
				}
			}
		case "likes":
			var ts []Track
			ts, err = a.Likes()
			items = trackItems(ts)
		case "history":
			var ts []Track
			ts, err = a.History()
			items = trackItems(ts)
		case "albums":
			var list []MyAlbum
			list, err = a.MyAlbums("")
			items = append(items, item{kind: kNewAlbum, title: "+ новый альбом"})
			for _, x := range list {
				items = append(items, item{kind: kMyAlbum, num: x.ID, title: x.Name, sub: fmt.Sprintf("%d тр.", x.Count)})
			}
		case "myalbum":
			var al *MyAlbum
			if al, err = a.MyAlbum(v.num); err == nil {
				items = trackItems(al.Tracks)
			}
		case "album":
			var al *Album
			if al, err = a.Album(v.id); err == nil {
				items = append(items, item{kind: kText, title: strings.TrimSpace(fmt.Sprintf("%s · %s %s", al.Type, Track{Artists: al.Artists}.ArtistNames(), al.Year))})
				items = append(items, trackItems(al.Tracks)...)
			}
		case "playlist":
			var pl *Playlist
			if pl, err = a.Playlist(v.id); err == nil {
				items = trackItems(pl.Tracks)
			}
		case "artist":
			var ar *ArtistPage
			if ar, err = a.Artist(v.id); err == nil {
				if len(ar.Songs.Results) > 0 {
					items = append(items, header("Треки"))
					items = append(items, trackItems(ar.Songs.Results)...)
				}
				for _, sec := range []struct {
					name string
					rs   []SearchResult
				}{{"Альбомы", ar.Albums.Results}, {"Синглы", ar.Singles.Results}} {
					if len(sec.rs) == 0 {
						continue
					}
					items = append(items, header(sec.name))
					for _, r := range sec.rs {
						items = append(items, item{kind: kAlbum, id: r.BrowseID, title: r.Title, sub: r.Year})
					}
				}
			}
		case "lyrics":
			var text, src string
			text, src, err = a.Lyrics(v.id)
			if err == nil && text == "" {
				err = errors.New("текста нет")
			}
			for _, l := range strings.Split(text, "\n") {
				items = append(items, item{kind: kText, title: l})
			}
			if src != "" {
				items = append(items, item{kind: kText}, item{kind: kText, title: src})
			}
		case "picker":
			var list []MyAlbum
			list, err = a.MyAlbums(v.target.VideoID)
			items = append(items, item{kind: kNewAlbum, title: "+ новый альбом с этим треком"})
			for _, x := range list {
				has := x.Has
				items = append(items, item{kind: kMyAlbum, num: x.ID, title: x.Name, sub: fmt.Sprintf("%d тр.", x.Count), check: &has})
			}
		}
		return loadedMsg{v, items, err}
	}
}

// ---------- update ----------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.search.Width = msg.Width - 6
	case tickMsg:
		return m, tea.Batch(tick(), fetchStatus)
	case statusMsg:
		m.st, m.stErr = msg.st, msg.err
		if msg.err == nil && msg.st.QVer != m.qver {
			m.qver = msg.st.QVer
			return m, fetchQueue
		}
	case queueMsg:
		if msg.err == nil {
			v := m.stacks[tabQueue][0]
			items := trackItems(msg.q.Queue)
			for i := range items {
				items[i].num = i
			}
			first := !v.loaded
			v.items, v.loaded = items, true
			if first || v.cursor >= len(items) {
				v.cursor = max(0, msg.q.Idx)
			}
			m.fix(v)
		}
	case likedMsg:
		m.liked = msg
	case loadedMsg:
		v := msg.v
		v.loading, v.loaded, v.items = false, true, msg.items
		if msg.err != nil {
			v.err = msg.err.Error()
		}
		if v.cursor >= len(v.items) {
			v.cursor = 0
		}
		m.fix(v)
	case noteMsg:
		if msg.err != nil {
			m.flash(msg.err.Error(), true)
		} else {
			m.flash(msg.text, false)
		}
	case tea.KeyMsg:
		return m, m.key(msg)
	}
	return m, nil
}

func (m *model) flash(s string, isErr bool) { m.msg, m.msgErr, m.msgAt = s, isErr, time.Now() }

func (m *model) listHeight() int { return max(1, m.h-5) }

// fix keeps the cursor on a selectable row and inside the visible window.
func (m *model) fix(v *view) {
	n := len(v.items)
	if n == 0 {
		v.cursor, v.offset = 0, 0
		return
	}
	v.cursor = min(max(v.cursor, 0), n-1)
	if v.kind != "lyrics" && !v.items[v.cursor].selectable() {
		for i := v.cursor; i < n; i++ {
			if v.items[i].selectable() {
				v.cursor = i
				break
			}
		}
	}
	h := m.listHeight()
	if v.kind == "search" || v.kind == "search-results" {
		h--
	}
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+h {
		v.offset = v.cursor - h + 1
	}
	// show the header above the first row
	if v.offset > 0 && v.offset == v.cursor && !v.items[v.offset-1].selectable() {
		v.offset--
	}
}

func (m *model) move(v *view, d int) {
	n := len(v.items)
	if n == 0 {
		return
	}
	if v.kind == "lyrics" {
		v.offset = min(max(v.offset+d, 0), max(0, n-m.listHeight()))
		return
	}
	step := 1
	if d < 0 {
		step = -1
	}
	// land d rows away, then skip headers in the direction of travel
	for i := min(max(v.cursor+d, 0), n-1); i >= 0 && i < n; i += step {
		if v.items[i].selectable() {
			v.cursor = i
			break
		}
	}
	m.fix(v)
}

func (m *model) selected() *item {
	v := m.cur()
	if v.cursor < len(v.items) && v.items[v.cursor].selectable() {
		return &v.items[v.cursor]
	}
	return nil
}

func (m *model) selTrack() *Track {
	if it := m.selected(); it != nil && it.track != nil {
		return it.track
	}
	return nil
}

func (m *model) playing() *Track {
	if m.st != nil {
		return m.st.Track
	}
	return nil
}

func (m *model) key(k tea.KeyMsg) tea.Cmd {
	if m.prompt != nil {
		switch k.String() {
		case "esc":
			m.prompt = nil
			return nil
		case "enter":
			p := m.prompt
			m.prompt = nil
			if s := strings.TrimSpace(p.input.Value()); s != "" {
				return p.submit(s)
			}
			return nil
		}
		var c tea.Cmd
		m.prompt.input, c = m.prompt.input.Update(k)
		return c
	}
	// fast input arrives as one multi-rune key ("jj"): handle each rune on its own
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 && !k.Paste && !m.typing {
		var cmds []tea.Cmd
		for _, r := range k.Runes {
			cmds = append(cmds, m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}))
		}
		return tea.Batch(cmds...)
	}
	v := m.cur()
	if m.typing {
		switch k.String() {
		case "esc":
			m.typing = false
			m.search.Blur()
			return nil
		case "enter":
			q := strings.TrimSpace(m.search.Value())
			m.typing = false
			m.search.Blur()
			if q == "" {
				return nil
			}
			m.stacks[1] = m.stacks[1][:1]
			return m.push(&view{kind: "search-results", title: "Поиск: " + q, id: q})
		case "ctrl+f":
			m.filter = (m.filter + 1) % len(searchFilters)
			return nil
		case "ctrl+c":
			return tea.Quit
		}
		var c tea.Cmd
		m.search, c = m.search.Update(k)
		return c
	}

	switch k.String() {
	case "ctrl+c", "q":
		return tea.Quit
	case "Q":
		_ = rpc("quit", nil, nil, false)
		return tea.Quit
	case "?":
		m.help = !m.help
	case "tab":
		return m.setTab((m.tab + 1) % len(tabNames))
	case "shift+tab":
		return m.setTab((m.tab + len(tabNames) - 1) % len(tabNames))
	case "1", "2", "3", "4", "5", "6":
		return m.setTab(int(k.String()[0] - '1'))
	case "/":
		m.setTab(1)
		m.stacks[1] = m.stacks[1][:1]
		m.typing = true
		m.search.SetValue("")
		return m.search.Focus()
	case "f":
		if v.kind == "search" || v.kind == "search-results" {
			m.filter = (m.filter + 1) % len(searchFilters)
			if v.kind == "search-results" {
				return m.load(v)
			}
		}
	case "up", "k":
		m.move(v, -1)
	case "down", "j":
		m.move(v, 1)
	case "pgup", "ctrl+u":
		m.move(v, -m.listHeight()/2)
	case "pgdown", "ctrl+d":
		m.move(v, m.listHeight()/2)
	case "home", "g":
		v.cursor, v.offset = 0, 0
		m.fix(v)
	case "end", "G":
		v.cursor = len(v.items) - 1
		m.fix(v)
	case "esc", "backspace", "h":
		m.pop()
	case "ctrl+r":
		if v.kind == "queue" {
			return fetchQueue
		}
		return m.load(v)
	case "enter", "o":
		return m.activate()
	case "a", "A":
		if t := m.selTrack(); t != nil {
			return do("add", Args{Tracks: []Track{*t}, Next: k.String() == "A"})
		}
		if it := m.selected(); it != nil && it.kind == kMyAlbum && v.kind != "picker" {
			return m.withTracks(it, func(ts []Track) tea.Cmd { return do("add", Args{Tracks: ts}) })
		}
	case "R":
		if t := m.selTrack(); t != nil {
			return m.radio(*t)
		}
	case "x", "delete":
		if v.kind == "queue" {
			if it := m.selected(); it != nil {
				return tea.Sequence(do("remove", Args{Index: it.num}), fetchQueue)
			}
		}
		if v.kind == "myalbum" {
			if t := m.selTrack(); t != nil {
				id, vid := v.num, t.VideoID
				return func() tea.Msg {
					if err := m.api.AlbumRemove(id, vid); err != nil {
						return noteMsg{err: err}
					}
					return m.load(v)()
				}
			}
		}
	case " ":
		return do("toggle", nil)
	case "n", ">":
		return do("next", nil)
	case "p", "<":
		return do("prev", nil)
	case "right", "l":
		return do("seek", Args{Value: "+10"})
	case "left":
		return do("seek", Args{Value: "-10"})
	case "+", "=":
		return do("volume", Args{Value: "+5"})
	case "-", "_":
		return do("volume", Args{Value: "-5"})
	case "L", "*":
		var t *Track
		if k.String() == "L" {
			t = m.selTrack()
		}
		return tea.Sequence(do("like", Args{Track: t}), fetchLiked)
	case "d":
		return tea.Sequence(do("dislike", nil), fetchLiked)
	case "w":
		m.flash("Собираю волну…", false)
		return do("wave", nil)
	case "m":
		mode := "mix"
		if m.st != nil {
			for i, md := range WaveModes {
				if md.ID == m.st.WaveMode {
					mode = WaveModes[(i+1)%len(WaveModes)].ID
				}
			}
		}
		return tea.Sequence(do("wavemode", Args{Value: mode}), note("Режим волны: "+waveModeName(mode)))
	case "r":
		return do("repeat", nil)
	case "s":
		return do("shuffle", nil)
	case "y":
		if t := m.playing(); t != nil {
			return m.push(&view{kind: "lyrics", title: "Текст: " + t.Label(), id: t.VideoID})
		}
	case "b":
		t := m.selTrack()
		if t == nil {
			t = m.playing()
		}
		if t != nil {
			tt := *t
			return m.push(&view{kind: "picker", title: "В альбом: " + tt.Label(), target: &tt})
		}
	case "D":
		t := m.selTrack()
		if t == nil {
			t = m.playing()
		}
		if t != nil {
			tt := *t
			dir := musicDir()
			m.flash("Качаю "+tt.Label()+"…", false)
			return func() tea.Msg {
				path, err := m.api.Download(tt, dir)
				if err != nil {
					return noteMsg{err: err}
				}
				return noteMsg{text: "Сохранено: " + path}
			}
		}
	}
	return nil
}

func musicDir() string {
	home, _ := os.UserHomeDir()
	d := filepath.Join(home, "Music")
	_ = os.MkdirAll(d, 0o755)
	return d
}

func (m *model) radio(t Track) tea.Cmd {
	m.flash("Радио: "+t.Label(), false)
	a := m.api
	return func() tea.Msg {
		ts, err := a.Radio(t.VideoID)
		if err != nil {
			return noteMsg{err: err}
		}
		if len(ts) == 0 || ts[0].VideoID != t.VideoID {
			ts = append([]Track{t}, ts...)
		}
		if err := rpc("play", Args{Tracks: ts}, nil, true); err != nil {
			return noteMsg{err: err}
		}
		return fetchStatus()
	}
}

func (m *model) withTracks(it *item, f func([]Track) tea.Cmd) tea.Cmd {
	a, id := m.api, it.num
	return func() tea.Msg {
		al, err := a.MyAlbum(id)
		if err != nil {
			return noteMsg{err: err}
		}
		return f(al.Tracks)()
	}
}

func (m *model) activate() tea.Cmd {
	v := m.cur()
	it := m.selected()
	if v.kind == "search" {
		m.typing = true
		return m.search.Focus()
	}
	if it == nil {
		return nil
	}
	switch it.kind {
	case kSong:
		if v.kind == "queue" {
			return do("jump", Args{Index: it.num})
		}
		var ts []Track
		at := 0
		for i := range v.items {
			if t := v.items[i].track; t != nil {
				if i == v.cursor {
					at = len(ts)
				}
				ts = append(ts, *t)
			}
		}
		// a lone song from search or the home feed: play it with its radio
		if v.kind == "search-results" || v.kind == "home" {
			return m.radio(*it.track)
		}
		return do("play", Args{Tracks: ts, Index: at})
	case kWaveMode:
		m.flash("Собираю волну…", false)
		return do("wave", Args{Value: it.id})
	case kAlbum:
		return m.push(&view{kind: "album", title: it.title, id: it.id})
	case kArtist:
		return m.push(&view{kind: "artist", title: it.title, id: it.id})
	case kPlaylist:
		return m.push(&view{kind: "playlist", title: it.title, id: it.id})
	case kMix:
		a, id := m.api, it.id
		m.flash("Микс: "+it.title, false)
		return func() tea.Msg {
			ts, err := a.PlaylistRadio(id)
			if err != nil {
				return noteMsg{err: err}
			}
			if err := rpc("play", Args{Tracks: ts}, nil, true); err != nil {
				return noteMsg{err: err}
			}
			return fetchStatus()
		}
	case kMyAlbum:
		if v.kind == "picker" {
			a, t, id, on := m.api, *v.target, it.num, !*it.check
			*it.check = on
			return func() tea.Msg {
				var err error
				if on {
					err = a.AlbumAdd(id, t)
				} else {
					err = a.AlbumRemove(id, t.VideoID)
				}
				if err != nil {
					return noteMsg{err: err}
				}
				if on {
					return noteMsg{text: "Добавлено в «" + it.title + "»"}
				}
				return noteMsg{text: "Убрано из «" + it.title + "»"}
			}
		}
		return m.push(&view{kind: "myalbum", title: it.title, num: it.num})
	case kNewAlbum:
		ti := textinput.New()
		ti.Prompt = "название: "
		ti.CharLimit = 100
		ti.Focus()
		target := v.target
		a := m.api
		m.prompt = &promptState{label: "Новый альбом", input: ti, submit: func(name string) tea.Cmd {
			return func() tea.Msg {
				id, err := a.CreateAlbum(name)
				if err == nil && target != nil {
					err = a.AlbumAdd(id, *target)
				}
				if err != nil {
					return noteMsg{err: err}
				}
				return m.load(v)()
			}
		}}
		return textinput.Blink
	}
	return nil
}

// ---------- view ----------

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) > w {
		return runewidth.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-runewidth.StringWidth(s))
}

func (m *model) View() string {
	if m.w == 0 {
		return ""
	}
	var b strings.Builder
	// tabs
	var tabs []string
	for i, n := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, n)
		if i == m.tab {
			tabs = append(tabs, sTabOn.Render(label))
		} else {
			tabs = append(tabs, sTabOff.Render(label))
		}
	}
	b.WriteString(sAccent.Render(" ≋ b.music  ") + strings.Join(tabs, "  ") + "\n")

	v := m.cur()
	title := v.title
	if s := m.stacks[m.tab]; len(s) > 1 {
		title = sDim.Render("← ") + title
	}
	status := ""
	if v.loading {
		status = sDim.Render("  загрузка…")
	} else if v.err != "" {
		status = "  " + sErr.Render(v.err)
	}
	b.WriteString(" " + sBold.Render(title) + status + "\n")

	h := m.listHeight()
	lines := []string{}
	if v.kind == "search" || v.kind == "search-results" {
		f := sDim.Render(fmt.Sprintf("  [f/ctrl+f] %s", searchFilters[m.filter].name))
		if m.typing {
			lines = append(lines, " "+m.search.View())
		} else {
			q := m.search.Value()
			if q == "" {
				q = sDim.Render("нажми / или enter, чтобы искать")
			}
			lines = append(lines, " / "+q+f)
		}
		h--
	}
	lines = append(lines, m.renderList(v, h)...)
	for len(lines) < m.listHeight() {
		lines = append(lines, "")
	}
	b.WriteString(strings.Join(lines[:m.listHeight()], "\n") + "\n")
	b.WriteString(sDim.Render(strings.Repeat("─", m.w)) + "\n")
	b.WriteString(m.renderNow() + "\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m *model) renderList(v *view, h int) []string {
	var out []string
	if len(v.items) == 0 && v.loaded && v.err == "" && v.kind != "search" {
		return []string{sDim.Render("  пусто")}
	}
	if v.kind == "search" && len(v.items) == 0 {
		return []string{"", sDim.Render("  enter — в выдаче играет радио от трека, a — в очередь, R — радио")}
	}
	if m.help {
		return helpLines()
	}
	playingID := ""
	if t := m.playing(); t != nil {
		playingID = t.VideoID
	}
	w := m.w
	for i := v.offset; i < len(v.items) && len(out) < h; i++ {
		it := v.items[i]
		switch it.kind {
		case kHeader:
			out = append(out, " "+sHeader.Render(fit(it.title, w-2)))
			continue
		case kText:
			out = append(out, "  "+sDim.Render(fit(it.title, w-3)))
			continue
		}
		mark := "  "
		if it.track != nil && it.track.VideoID == playingID {
			mark = sAccent.Render("▶ ")
		} else if it.check != nil {
			if *it.check {
				mark = sAccent.Render("✓ ")
			} else {
				mark = "· "
			}
		}
		heart := " "
		if it.track != nil && m.liked[it.track.VideoID] {
			heart = sAccent.Render("♥")
		}
		num := ""
		if v.kind == "queue" {
			num = fmt.Sprintf("%3d ", it.num+1)
		}
		dur := ""
		if it.track != nil {
			dur = it.track.Duration
		}
		rest := w - 4 - len(num) - 2 - 7
		tw := rest * 55 / 100
		sw := rest - tw - 1
		line := mark + num + heart + " " + fit(it.title, tw) + " " + sDim.Render(fit(it.sub, sw)) + sDim.Render(fmt.Sprintf("%7s", dur))
		if i == v.cursor && v.kind != "lyrics" {
			line = sSel.Render(line + strings.Repeat(" ", max(0, w-lipgloss.Width(line))))
		}
		out = append(out, line)
	}
	if m.prompt != nil {
		out = append(out, "", " "+sBold.Render(m.prompt.label)+"  "+m.prompt.input.View())
	}
	return out
}

func helpLines() []string {
	return strings.Split(`  Навигация   ↑↓/jk  pgup/pgdn  g/G   enter — играть/открыть   esc/h — назад
              tab, 1-6 — вкладки   / — поиск (f — тип)   ctrl+r — обновить
  Плеер       space — пауза   n/p — след./пред.   ←/→ — ∓10с   +/- — громкость
              r — повтор   s — перемешать   w — волна   m — режим волны
  Трек        * — ♥ текущему   L — ♥ выбранному   d — не нравится (текущий)
              a — в очередь   A — следующим   R — радио от трека   b — в мой альбом
              y — текст песни   D — скачать в ~/Music   x — убрать из очереди/альбома
  Выход       q — закрыть TUI (музыка играет дальше)   Q — выйти и остановить
  CLI         bmusic --help: status, next, like, wave… — для хоткеев niri`, "\n")
}

func (m *model) renderNow() string {
	s := m.st
	if m.stErr != nil {
		return " " + sErr.Render("плеер: "+m.stErr.Error())
	}
	if s == nil || s.Track == nil {
		return sDim.Render(" ничего не играет — w: моя волна, /: поиск")
	}
	icon := "▶"
	if s.Paused {
		icon = "⏸"
	}
	if s.Loading && s.Playing {
		icon = "…"
	}
	heart := ""
	if s.Liked {
		heart = sAccent.Render(" ♥")
	}
	var flags []string
	if s.Wave != "" {
		flags = append(flags, "≋ "+waveModeName(s.Wave))
	}
	if s.Repeat == "all" {
		flags = append(flags, "⟳ все")
	} else if s.Repeat == "one" {
		flags = append(flags, "⟳ 1")
	}
	if s.Shuffle {
		flags = append(flags, "⤮")
	}
	flags = append(flags, fmt.Sprintf("♪ %.0f%%", s.Volume), fmt.Sprintf("%d/%d", s.Index+1, s.Length))
	right := strings.Join(flags, "  ")
	times := fmt.Sprintf(" %s / %s ", fmtTime(s.Pos), fmtTime(s.Dur))

	barW := 24
	if m.w > 120 {
		barW = 36
	}
	fill := 0
	if s.Dur > 0 {
		fill = min(barW, int(float64(barW)*s.Pos/s.Dur))
	}
	bar := sAccent.Render(strings.Repeat("━", fill)) + sDim.Render(strings.Repeat("─", barW-fill))

	left := " " + sAccent.Render(icon) + " " + sBold.Render(s.Track.Title) + sDim.Render(" — "+s.Track.ArtistNames()) + heart
	fixed := lipgloss.Width(times) + barW + lipgloss.Width(right) + 3
	room := m.w - fixed
	if lipgloss.Width(left) > room {
		left = " " + icon + " " + fit(s.Track.Label(), max(5, room-4))
	}
	pad := max(1, m.w-lipgloss.Width(left)-fixed+1)
	return left + strings.Repeat(" ", pad) + bar + times + sDim.Render(right)
}

func (m *model) renderFooter() string {
	if m.msg != "" && time.Since(m.msgAt) < 4*time.Second {
		if m.msgErr {
			return " " + sErr.Render(fit(m.msg, m.w-2))
		}
		return " " + sAccent.Render(fit(m.msg, m.w-2))
	}
	if m.st != nil && m.st.Msg != "" {
		return " " + sAccent.Render(fit(m.st.Msg, m.w-2))
	}
	if m.st != nil && m.st.Track != nil && m.st.Track.Reason != "" {
		return " " + sDim.Render(fit("≋ "+m.st.Track.Reason, m.w-2))
	}
	return sDim.Render(fit(" ? — помощь   space — пауза   n/p — треки   w — волна   * — ♥   / — поиск   q — выход", m.w))
}
