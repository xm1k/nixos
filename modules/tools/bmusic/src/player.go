package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// State is what survives a daemon restart.
type State struct {
	Queue    []Track `json:"queue"`
	Idx      int     `json:"idx"`
	Wave     string  `json:"wave"` // active wave mode, "" when playing a plain list
	WaveMode string  `json:"waveMode"`
	Repeat   string  `json:"repeat"` // off | all | one
	Shuffle  bool    `json:"shuffle"`
	Volume   float64 `json:"volume"`
	Pos      float64 `json:"pos"`
}

type Status struct {
	Track    *Track  `json:"track"`
	Index    int     `json:"index"`
	Length   int     `json:"length"`
	Pos      float64 `json:"pos"`
	Dur      float64 `json:"dur"`
	Playing  bool    `json:"playing"` // a track is loaded
	Paused   bool    `json:"paused"`
	Volume   float64 `json:"volume"`
	Wave     string  `json:"wave"`
	WaveMode string  `json:"waveMode"`
	Repeat   string  `json:"repeat"`
	Shuffle  bool    `json:"shuffle"`
	Liked    bool    `json:"liked"`
	Loading  bool    `json:"loading"`
	Msg      string  `json:"msg"`
	QVer     int     `json:"qver"`
}

type listen struct {
	t      Track
	played float64
	last   float64
}

type Player struct {
	mu  sync.Mutex
	api *API
	mpv *MPV
	st  State

	loaded     bool
	pos, dur   float64
	paused     bool
	entry      int64 // mpv playlist entry of the current track
	seekOnLoad float64
	retries    int
	buffering  bool
	prepared   string

	listen         *listen
	switchHow      string
	waveSkips      int
	pendingRefresh bool
	lastEvent      chan struct{}
	extending      bool

	likes map[string]bool
	msg   string
	msgAt time.Time
	qver  int
	saved time.Time
}

func statePath() string { return filepath.Join(stateDir(), "state.json") }

func NewPlayer() *Player {
	p := &Player{api: NewAPI(), likes: map[string]bool{}, st: State{Idx: -1, WaveMode: "mix", Repeat: "off", Volume: 80}}
	if b, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(b, &p.st)
	}
	if p.st.Idx >= len(p.st.Queue) {
		p.st.Idx = len(p.st.Queue) - 1
	}
	go p.loadLikes()
	return p
}

func (p *Player) loadLikes() {
	ls, err := p.api.Likes()
	if err != nil {
		log.Printf("likes: %v", err)
		if errors.Is(err, ErrAuth) {
			p.mu.Lock()
			p.say(ErrAuth.Error())
			p.mu.Unlock()
		}
		return
	}
	p.mu.Lock()
	p.likes = map[string]bool{}
	for _, t := range ls {
		p.likes[t.VideoID] = true
	}
	p.mu.Unlock()
}

// ---------- helpers (call with p.mu held) ----------

func (p *Player) say(m string) { p.msg, p.msgAt = m, time.Now(); log.Print(m) }

func (p *Player) cur() *Track {
	if p.st.Idx >= 0 && p.st.Idx < len(p.st.Queue) {
		return &p.st.Queue[p.st.Idx]
	}
	return nil
}

func (p *Player) save() {
	p.st.Pos = p.pos
	q := p.st
	if len(q.Queue) > 500 {
		// keep the window around the current track
		lo := max(0, q.Idx-250)
		hi := min(len(q.Queue), lo+500)
		q.Queue, q.Idx = q.Queue[lo:hi], q.Idx-lo
	}
	b, _ := json.Marshal(q)
	_ = os.MkdirAll(stateDir(), 0o700)
	tmp := statePath() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, statePath())
	}
	p.saved = time.Now()
}

func (p *Player) changed() { p.qver++; p.save() }

func (p *Player) ensureMPV() error {
	if p.mpv != nil && p.mpv.Alive() {
		return nil
	}
	p.api.Token = loadToken()
	m, err := StartMPV(p.api.Token, p.st.Volume)
	if err != nil {
		return err
	}
	p.mpv = m
	p.loaded = false
	go p.events(m)
	return nil
}

// ---------- playback ----------

func (p *Player) playAt(i int, pos float64) error {
	if i < 0 || i >= len(p.st.Queue) {
		return errors.New("нет такого трека в очереди")
	}
	if err := p.ensureMPV(); err != nil {
		return err
	}
	p.finalizeListen()
	p.st.Idx = i
	t := p.st.Queue[i]
	p.listen = &listen{t: t, last: -1}
	p.retries = 0
	if err := p.load(t, pos); err != nil {
		return err
	}
	go func() { _ = p.api.AddHistory(t) }()
	if p.pendingRefresh {
		p.pendingRefresh = false
		p.say("Волна подстраивается под вас")
		go p.refreshWave(0)
	} else if p.st.Idx >= len(p.st.Queue)-3 {
		go p.extend()
	}
	p.changed()
	return nil
}

func (p *Player) load(t Track, pos float64) error {
	data, err := p.mpv.Command("loadfile", p.api.StreamURL(t.VideoID), "replace")
	if err != nil {
		return err
	}
	var r struct {
		Entry int64 `json:"playlist_entry_id"`
	}
	_ = json.Unmarshal(data, &r)
	p.entry = r.Entry
	p.seekOnLoad = pos
	p.loaded, p.pos, p.dur, p.buffering = true, pos, t.Seconds(), true
	_ = p.mpv.Set("pause", false)
	return nil
}

func (p *Player) next(auto bool) error {
	if auto && p.st.Repeat == "one" {
		p.switchHow = "ended"
		return p.playAt(p.st.Idx, 0)
	}
	if auto {
		p.switchHow = "ended"
	} else {
		p.switchHow = "next"
	}
	switch {
	case p.st.Idx < len(p.st.Queue)-1:
		return p.playAt(p.st.Idx+1, 0)
	case p.st.Repeat == "all" && len(p.st.Queue) > 0:
		return p.playAt(0, 0)
	case auto || p.st.Wave != "":
		go func() {
			p.extend()
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.st.Idx < len(p.st.Queue)-1 {
				_ = p.playAt(p.st.Idx+1, 0)
			} else {
				p.finalizeListen()
				p.loaded = false
				p.say("Очередь закончилась")
			}
		}()
	default:
		p.say("Это последний трек в очереди")
	}
	return nil
}

func (p *Player) prev() error {
	if !p.loaded {
		return p.playAt(max(p.st.Idx, 0), 0)
	}
	if p.pos > 4 || p.st.Idx <= 0 {
		_, err := p.mpv.Command("seek", 0, "absolute")
		return err
	}
	return p.playAt(p.st.Idx-1, 0)
}

func (p *Player) playList(ts []Track, i int) error {
	ts = validTracks(append([]Track(nil), ts...))
	if len(ts) == 0 {
		return errors.New("пустой список")
	}
	i = min(max(i, 0), len(ts)-1)
	p.st.Wave = ""
	if p.st.Shuffle {
		first := ts[i]
		rest := append(append([]Track(nil), ts[:i]...), ts[i+1:]...)
		rand.Shuffle(len(rest), func(a, b int) { rest[a], rest[b] = rest[b], rest[a] })
		ts, i = append([]Track{first}, rest...), 0
	}
	p.st.Queue = ts
	return p.playAt(i, 0)
}

func (p *Player) add(ts []Track, next bool) error {
	ts = validTracks(ts)
	if len(ts) == 0 {
		return errors.New("нечего добавлять")
	}
	if len(p.st.Queue) == 0 {
		return p.playList(ts, 0)
	}
	if next {
		at := p.st.Idx + 1
		q := append([]Track(nil), p.st.Queue[:at]...)
		q = append(q, ts...)
		p.st.Queue = append(q, p.st.Queue[at:]...)
	} else {
		p.st.Queue = append(p.st.Queue, ts...)
	}
	if len(ts) == 1 {
		p.say("В очереди: " + ts[0].Label())
	} else {
		p.say(fmt.Sprintf("В очередь добавлено %d", len(ts)))
	}
	p.changed()
	return nil
}

func (p *Player) toggle() error {
	if !p.loaded || p.mpv == nil || !p.mpv.Alive() {
		if len(p.st.Queue) == 0 {
			return errors.New("очередь пуста — запусти волну или найди трек")
		}
		return p.playAt(max(p.st.Idx, 0), p.st.Pos)
	}
	return p.mpv.Set("pause", !p.paused)
}

func (p *Player) setPause(v bool) error {
	if !p.loaded {
		if !v {
			return p.toggle()
		}
		return nil
	}
	return p.mpv.Set("pause", v)
}

func parseTime(s string) (float64, error) {
	var t float64
	for _, part := range strings.Split(s, ":") {
		x, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0, fmt.Errorf("не понял время %q", s)
		}
		t = t*60 + x
	}
	return t, nil
}

func (p *Player) seek(spec string) error {
	if !p.loaded {
		return errors.New("ничего не играет")
	}
	mode := "absolute"
	if strings.HasPrefix(spec, "+") || strings.HasPrefix(spec, "-") {
		mode = "relative"
	}
	sign := 1.0
	if strings.HasPrefix(spec, "-") {
		sign = -1
	}
	v, err := parseTime(strings.TrimLeft(spec, "+-"))
	if err != nil {
		return err
	}
	_, err = p.mpv.Command("seek", sign*v, mode)
	return err
}

func (p *Player) volume(spec string) error {
	v := p.st.Volume
	x, err := strconv.ParseFloat(strings.TrimLeft(spec, "+"), 64)
	if err != nil {
		return fmt.Errorf("громкость: %q", spec)
	}
	if strings.HasPrefix(spec, "+") || strings.HasPrefix(spec, "-") {
		v += x
	} else {
		v = x
	}
	v = min(max(v, 0), 130)
	p.st.Volume = v
	if p.mpv != nil && p.mpv.Alive() {
		_ = p.mpv.Set("volume", v)
	}
	p.save()
	return nil
}

// ---------- listening signals & my wave (mirrors the web player) ----------

func (p *Player) finalizeListen() {
	how := p.switchHow
	if how == "" {
		how = "switch"
	}
	l := p.listen
	p.listen, p.switchHow = nil, ""
	if l == nil {
		return
	}
	dur := p.dur
	if dur <= 0 {
		dur = l.t.Seconds()
	}
	var kind string
	switch {
	case how == "ended" || (dur > 0 && l.played >= 0.8*dur):
		kind = "complete"
	case l.played >= 30:
		kind = "partial"
	case how == "next":
		kind = "skip"
	default:
		return // switched away early: too ambiguous to count
	}
	done := make(chan struct{})
	p.lastEvent = done
	t, played := l.t, l.played
	go func() {
		defer close(done)
		if err := p.api.Event(t, kind, played); err != nil {
			log.Printf("event: %v", err)
		}
	}()
	if p.st.Wave == "" {
		return
	}
	if kind != "skip" {
		p.waveSkips = 0
	} else if p.waveSkips++; p.waveSkips >= 2 {
		p.waveSkips = 0
		p.pendingRefresh = true
	}
}

// extend tops up the queue: more wave, or radio from the current track. Call without p.mu.
func (p *Player) extend() {
	p.mu.Lock()
	wave := p.st.Wave
	p.mu.Unlock()
	if wave != "" {
		p.waveMore()
	} else {
		p.extendRadio()
	}
}

func (p *Player) appendNew(ts []Track) int {
	have := map[string]bool{}
	for _, t := range p.st.Queue {
		have[t.VideoID] = true
	}
	n := 0
	for _, t := range ts {
		if t.VideoID != "" && !have[t.VideoID] {
			have[t.VideoID] = true
			p.st.Queue = append(p.st.Queue, t)
			n++
		}
	}
	if n > 0 {
		p.changed()
	}
	return n
}

func (p *Player) extendRadio() {
	p.mu.Lock()
	t := p.cur()
	if t == nil || p.extending || p.st.Repeat != "off" {
		p.mu.Unlock()
		return
	}
	p.extending = true
	id := t.VideoID
	p.mu.Unlock()
	ts, err := p.api.Radio(id)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.extending = false
	if err != nil {
		log.Printf("radio: %v", err)
		return
	}
	p.appendNew(ts)
}

func (p *Player) waveMore() int {
	p.mu.Lock()
	if p.st.Wave == "" || p.extending {
		p.mu.Unlock()
		return 0
	}
	p.extending = true
	mode := p.st.Wave
	q := p.st.Queue[max(0, len(p.st.Queue)-150):]
	ex := make([]string, len(q))
	for i, t := range q {
		ex[i] = t.VideoID
	}
	p.mu.Unlock()
	r, err := p.api.Wave(mode, ex)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.extending = false
	if err != nil {
		log.Printf("wave: %v", err)
		return 0
	}
	return p.appendNew(r.Tracks)
}

// refreshWave drops the upcoming wave tracks and rebuilds them with fresh signals.
func (p *Player) refreshWave(keep int) {
	p.mu.Lock()
	if p.st.Wave == "" {
		p.mu.Unlock()
		return
	}
	if cut := p.st.Idx + 1 + keep; cut < len(p.st.Queue) {
		p.st.Queue = p.st.Queue[:cut]
		p.changed()
	}
	ev := p.lastEvent
	p.mu.Unlock()
	if ev != nil {
		select {
		case <-ev:
		case <-time.After(5 * time.Second):
		}
	}
	p.waveMore()
}

func (p *Player) startWave(mode string) error {
	p.mu.Lock()
	if mode == "" {
		mode = p.st.WaveMode
	}
	p.say("Собираю волну…")
	p.mu.Unlock()
	r, err := p.api.Wave(mode, nil)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		return err
	}
	if len(r.Tracks) == 0 {
		return errors.New("не удалось собрать волну")
	}
	p.st.Wave, p.st.WaveMode, p.waveSkips, p.pendingRefresh = mode, mode, 0, false
	p.st.Queue = r.Tracks
	p.say("Моя волна: " + waveModeName(mode))
	return p.playAt(0, 0)
}

func (p *Player) setWaveMode(mode string) {
	p.st.WaveMode = mode
	if p.st.Wave != "" {
		p.st.Wave = mode
		p.say("Режим: " + waveModeName(mode))
		go p.refreshWave(0)
	}
	p.save()
}

func (p *Player) toggleLike(t *Track) (bool, error) {
	if t == nil {
		t = p.cur()
	}
	if t == nil {
		return false, errors.New("ничего не играет")
	}
	tr := *t
	if p.likes[tr.VideoID] {
		delete(p.likes, tr.VideoID)
		p.say("Убрано из любимого")
		go func() { _ = p.api.Unlike(tr.VideoID) }()
		return false, nil
	}
	p.likes[tr.VideoID] = true
	p.say("♥ " + tr.Label())
	done := make(chan struct{})
	p.lastEvent = done
	go func() { defer close(done); _ = p.api.Like(tr) }()
	return true, nil
}

func (p *Player) dislike(t *Track) error {
	c := p.cur()
	if t == nil {
		t = c
	}
	if t == nil {
		return errors.New("ничего не играет")
	}
	tr := *t
	delete(p.likes, tr.VideoID)
	done := make(chan struct{})
	p.lastEvent = done
	go func() { defer close(done); _ = p.api.Dislike(tr) }()
	p.say("Больше не покажу этот трек")
	if c == nil || c.VideoID != tr.VideoID {
		return nil
	}
	p.listen = nil // explicit signal already sent, don't count it as a skip too
	if p.st.Wave != "" {
		p.pendingRefresh = true
	}
	return p.next(false)
}

func (p *Player) shuffleUpcoming() {
	p.st.Shuffle = !p.st.Shuffle
	if p.st.Shuffle && p.st.Idx+2 < len(p.st.Queue) {
		rest := p.st.Queue[p.st.Idx+1:]
		rand.Shuffle(len(rest), func(a, b int) { rest[a], rest[b] = rest[b], rest[a] })
	}
	if p.st.Shuffle {
		p.say("Перемешивание: вкл")
	} else {
		p.say("Перемешивание: выкл")
	}
	p.changed()
}

func (p *Player) setRepeat(m string) error {
	if m == "" {
		m = map[string]string{"off": "all", "all": "one", "one": "off"}[p.st.Repeat]
	}
	if m != "off" && m != "all" && m != "one" {
		return fmt.Errorf("repeat: off|all|one")
	}
	p.st.Repeat = m
	p.say("Повтор: " + map[string]string{"off": "выкл", "all": "очередь", "one": "трек"}[m])
	p.save()
	return nil
}

func (p *Player) remove(i int) error {
	if i < 0 || i >= len(p.st.Queue) {
		return errors.New("нет такого трека в очереди")
	}
	if i == p.st.Idx {
		return errors.New("это текущий трек — переключись дальше")
	}
	p.st.Queue = append(p.st.Queue[:i], p.st.Queue[i+1:]...)
	if i < p.st.Idx {
		p.st.Idx--
	}
	p.changed()
	return nil
}

func (p *Player) clearUpcoming() {
	if c := p.cur(); c != nil {
		p.st.Queue, p.st.Idx = []Track{*c}, 0
	} else {
		p.st.Queue, p.st.Idx = nil, -1
	}
	p.st.Wave = ""
	p.changed()
}

func (p *Player) status() Status {
	s := Status{
		Track: p.cur(), Index: p.st.Idx, Length: len(p.st.Queue), Pos: p.pos, Dur: p.dur,
		Playing: p.loaded, Paused: p.paused || !p.loaded, Volume: p.st.Volume,
		Wave: p.st.Wave, WaveMode: p.st.WaveMode, Repeat: p.st.Repeat, Shuffle: p.st.Shuffle,
		Loading: p.buffering || p.extending, QVer: p.qver,
	}
	if !p.loaded {
		s.Pos = p.st.Pos
	}
	if s.Track != nil {
		s.Liked = p.likes[s.Track.VideoID]
		if s.Dur <= 0 {
			s.Dur = s.Track.Seconds()
		}
	}
	if time.Since(p.msgAt) < 4*time.Second {
		s.Msg = p.msg
	}
	return s
}

// ---------- mpv events ----------

func (p *Player) events(m *MPV) {
	for ev := range m.Events {
		p.mu.Lock()
		if p.mpv == m {
			p.handle(ev)
		}
		p.mu.Unlock()
	}
	p.mu.Lock()
	if p.mpv == m {
		p.loaded = false
		log.Print("mpv exited")
	}
	p.mu.Unlock()
}

func (p *Player) handle(ev mpvEvent) {
	switch ev.Event {
	case "property-change":
		switch ev.Name {
		case "time-pos":
			var v *float64
			if json.Unmarshal(ev.Data, &v) != nil || v == nil {
				return
			}
			if l := p.listen; l != nil {
				if d := *v - l.last; l.last >= 0 && d > 0 && d < 2 {
					l.played += d
				}
				l.last = *v
			}
			p.pos = *v
			if p.loaded && time.Since(p.saved) > 10*time.Second {
				p.save()
			}
		case "pause":
			_ = json.Unmarshal(ev.Data, &p.paused)
		case "duration":
			var v *float64
			if json.Unmarshal(ev.Data, &v) == nil && v != nil && *v > 0 {
				p.dur = *v
			}
		case "volume":
			var v *float64
			if json.Unmarshal(ev.Data, &v) == nil && v != nil {
				p.st.Volume = *v
			}
		}
	case "file-loaded":
		if p.seekOnLoad > 0 {
			_, _ = p.mpv.Command("seek", p.seekOnLoad, "absolute")
			p.seekOnLoad = 0
		}
	case "playback-restart":
		p.buffering = false
		if p.st.Idx+1 < len(p.st.Queue) && p.st.Queue[p.st.Idx+1].VideoID != p.prepared {
			id := p.st.Queue[p.st.Idx+1].VideoID
			p.prepared = id
			go func() { _ = p.api.Prepare(id) }()
		}
	case "end-file":
		if ev.Entry != 0 && p.entry != 0 && ev.Entry != p.entry {
			return // a track we already replaced
		}
		c := p.cur()
		switch ev.Reason {
		case "eof":
			// the stream broke off early: resume where it stopped
			if c != nil && p.dur > 30 && p.pos < p.dur-5 && p.retries < 3 {
				p.retries++
				log.Printf("premature eof at %.0f/%.0f, retry %d", p.pos, p.dur, p.retries)
				_ = p.load(*c, p.pos)
				return
			}
			p.loaded = false
			_ = p.next(true)
		case "error":
			if c != nil && p.retries < 2 {
				p.retries++
				log.Printf("playback error %q, retry %d", ev.Error, p.retries)
				_ = p.load(*c, p.pos)
				return
			}
			p.loaded = false
			p.say("Не удалось воспроизвести трек, дальше")
			p.listen = nil
			_ = p.next(true)
		}
	}
}

// ---------- requests ----------

type Request struct {
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}

type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

type Args struct {
	Tracks []Track `json:"tracks,omitempty"`
	Track  *Track  `json:"track,omitempty"`
	Index  int     `json:"index,omitempty"`
	Next   bool    `json:"next,omitempty"`
	Value  string  `json:"value,omitempty"`
}

func (p *Player) Handle(req Request) (any, error) {
	var a Args
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return nil, err
		}
	}
	// wave start does network I/O first, so it manages the lock itself
	if req.Cmd == "wave" {
		if a.Value != "" && !validMode(a.Value) {
			return nil, errors.New("режим: mix | favorites | discover")
		}
		return nil, p.startWave(a.Value)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch req.Cmd {
	case "status":
		return p.status(), nil
	case "queue":
		return map[string]any{"queue": p.st.Queue, "idx": p.st.Idx, "qver": p.qver}, nil
	case "play":
		return nil, p.playList(a.Tracks, a.Index)
	case "add":
		return nil, p.add(a.Tracks, a.Next)
	case "toggle":
		return nil, p.toggle()
	case "pause":
		return nil, p.setPause(true)
	case "resume":
		return nil, p.setPause(false)
	case "next":
		return nil, p.next(false)
	case "prev":
		return nil, p.prev()
	case "jump":
		return nil, p.playAt(a.Index, 0)
	case "remove":
		return nil, p.remove(a.Index)
	case "clear":
		p.clearUpcoming()
		return nil, nil
	case "seek":
		return nil, p.seek(a.Value)
	case "volume":
		return nil, p.volume(a.Value)
	case "wavemode":
		if !validMode(a.Value) {
			return nil, errors.New("режим: mix | favorites | discover")
		}
		p.setWaveMode(a.Value)
		return nil, nil
	case "like":
		return p.toggleLike(a.Track)
	case "dislike":
		return nil, p.dislike(a.Track)
	case "liked":
		return p.likes, nil
	case "repeat":
		return nil, p.setRepeat(a.Value)
	case "shuffle":
		p.shuffleUpcoming()
		return nil, nil
	case "reload":
		p.api.Token = loadToken()
		if p.mpv != nil {
			p.mpv.Quit()
			p.mpv, p.loaded = nil, false
		}
		go p.loadLikes()
		return nil, nil
	case "quit":
		p.finalizeListen()
		p.save()
		if p.mpv != nil {
			p.mpv.Quit()
		}
		ev := p.lastEvent
		go func() {
			if ev != nil {
				select {
				case <-ev:
				case <-time.After(3 * time.Second):
				}
			}
			os.Remove(socketPath())
			os.Exit(0)
		}()
		return nil, nil
	}
	return nil, fmt.Errorf("неизвестная команда %q", req.Cmd)
}

func validMode(m string) bool {
	for _, x := range WaveModes {
		if x.ID == m {
			return true
		}
	}
	return false
}
