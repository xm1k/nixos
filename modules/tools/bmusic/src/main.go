package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var videoIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func main() {
	root := &cobra.Command{
		Use:           "bmusic",
		Short:         "music.b.net в терминале: TUI и управление плеером",
		Long:          "Без аргументов открывает TUI. Играет фоновый демон (mpv), он переживает закрытие TUI;\nвсе команды ниже управляют им.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          func(*cobra.Command, []string) error { return runTUI() },
	}
	root.AddCommand(
		cmdLogin(), cmdLogout(), cmdWhoami(),
		cmdPlay(), cmdAdd(), cmdWave(), cmdSearch(),
		simple("toggle", "Пауза / продолжить", "toggle"),
		simple("pause", "Пауза", "pause"),
		simple("resume", "Продолжить", "resume"),
		simple("next", "Следующий трек", "next"),
		simple("prev", "Предыдущий трек (или в начало)", "prev"),
		simple("shuffle", "Перемешивание вкл/выкл (перемешивает и оставшуюся очередь)", "shuffle"),
		cmdSeek(), cmdVolume(), cmdRepeat(), cmdStatus(), cmdQueue(), cmdJump(),
		cmdLike(), simple("dislike", "Не нравится: убрать и больше не предлагать", "dislike"),
		cmdLibrary("likes", "Любимые треки", func(a *API) ([]Track, error) { return a.Likes() }),
		cmdLibrary("history", "История прослушивания", func(a *API) ([]Track, error) { return a.History() }),
		cmdLibrary("dislikes", "Треки с «не нравится»", func(a *API) ([]Track, error) { return a.Dislikes() }),
		cmdAlbums(), cmdLyrics(), cmdDownload(),
		&cobra.Command{Use: "tui", Short: "Открыть TUI", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { return runTUI() }},
		&cobra.Command{Use: "stop", Short: "Остановить плеер-демон", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			if err := rpc("quit", nil, nil, false); err != nil && !errors.Is(err, errNoDaemon) {
				return err
			}
			return nil
		}},
		&cobra.Command{Use: "daemon", Hidden: true, Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { return runDaemon() }},
	)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bmusic:", err)
		os.Exit(1)
	}
}

func simple(use, short, cmd string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		if err := rpc(cmd, nil, nil, true); err != nil {
			return err
		}
		return printNow()
	}}
}

// printNow shows the track that is playing after a command, briefly.
func printNow() error {
	s, err := getStatus()
	if err != nil || s.Track == nil {
		return nil
	}
	state := "▶"
	if s.Paused {
		state = "⏸"
	}
	fmt.Printf("%s %s\n", state, s.Track.Label())
	return nil
}

// ---------- auth ----------

func cmdLogin() *cobra.Command {
	var user string
	c := &cobra.Command{Use: "login", Short: "Войти учёткой b.net", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		in := bufio.NewReader(os.Stdin)
		if user == "" {
			fmt.Print("Логин: ")
			l, _ := in.ReadString('\n')
			user = strings.TrimSpace(l)
		}
		fmt.Print("Пароль: ")
		var pass string
		if term.IsTerminal(int(os.Stdin.Fd())) {
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return err
			}
			pass = string(b)
		} else {
			l, _ := in.ReadString('\n')
			pass = strings.TrimRight(l, "\r\n")
		}
		tok, err := Login(user, pass)
		if err != nil {
			return err
		}
		if err := saveToken(tok); err != nil {
			return err
		}
		a := NewAPI()
		me, err := a.Me()
		if err != nil {
			return err
		}
		_ = rpc("reload", nil, nil, false)
		fmt.Println("Вошли как", me)
		return nil
	}}
	c.Flags().StringVarP(&user, "user", "u", "", "логин")
	return c
}

func cmdLogout() *cobra.Command {
	return &cobra.Command{Use: "logout", Short: "Забыть сессию", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		_ = rpc("quit", nil, nil, false)
		if err := os.Remove(tokenFile()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}}
}

func cmdWhoami() *cobra.Command {
	return &cobra.Command{Use: "whoami", Short: "Пользователь и сводка вкуса", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		a := NewAPI()
		me, err := a.Me()
		if err != nil {
			return err
		}
		fmt.Println(me)
		if t, err := a.Taste(); err == nil {
			fmt.Printf("любимых: %d, наслушано: %.1f ч\n", t.Likes, t.Hours)
			var names []string
			for _, x := range t.Artists {
				names = append(names, x.Name)
			}
			if len(names) > 0 {
				fmt.Println("чаще всего:", strings.Join(names, ", "))
			}
		}
		return nil
	}}
}

// ---------- playing ----------

// trackFrom turns a video id, a YouTube Music link or a search query into a track.
func trackFrom(a *API, arg string) (*Track, error) {
	if i := strings.Index(arg, "v="); i >= 0 && strings.Contains(arg, "youtu") {
		arg = arg[i+2:]
		if j := strings.IndexAny(arg, "&#"); j >= 0 {
			arg = arg[:j]
		}
	}
	if videoIDRe.MatchString(arg) {
		if r, err := a.Radio(arg); err == nil && len(r) > 0 && r[0].VideoID == arg {
			return &r[0], nil
		}
	}
	ts, err := a.SearchSongs(arg, 5)
	if err != nil {
		return nil, err
	}
	if len(ts) == 0 {
		return nil, fmt.Errorf("ничего не нашлось: %s", arg)
	}
	return &ts[0], nil
}

func cmdPlay() *cobra.Command {
	var album, playlist, artist string
	var list, shuffle bool
	c := &cobra.Command{
		Use:   "play [запрос | videoId | ссылка]",
		Short: "Играть: без аргументов — продолжить; запрос — радио от лучшего совпадения",
		Example: `  bmusic play                      # продолжить
  bmusic play radiohead creep      # радио от найденного трека
  bmusic play -l кино              # очередь из результатов поиска
  bmusic play --album MPREb_xxxx   # альбом (id из bmusic search -t albums)`,
		RunE: func(_ *cobra.Command, args []string) error {
			a := NewAPI()
			q := strings.Join(args, " ")
			var ts []Track
			var err error
			switch {
			case album != "":
				var al *Album
				if al, err = a.Album(album); err == nil {
					ts = al.Tracks
				}
			case playlist != "":
				var pl *Playlist
				if pl, err = a.Playlist(playlist); err == nil {
					ts = pl.Tracks
				}
			case artist != "":
				var ar *ArtistPage
				if ar, err = a.Artist(artist); err == nil {
					ts = ar.Songs.Results
				}
			case q == "":
				if err := rpc("resume", nil, nil, true); err != nil {
					return err
				}
				return printNow()
			case list:
				ts, err = a.SearchSongs(q, 40)
			default:
				var t *Track
				if t, err = trackFrom(a, q); err == nil {
					ts = []Track{*t}
					if r, err := a.Radio(t.VideoID); err == nil && len(r) > 0 {
						if r[0].VideoID != t.VideoID {
							r = append([]Track{*t}, r...)
						}
						ts = r
					}
				}
			}
			if err != nil {
				return err
			}
			if len(ts) == 0 {
				return errors.New("ничего не нашлось")
			}
			if shuffle {
				rand.Shuffle(len(ts), func(i, j int) { ts[i], ts[j] = ts[j], ts[i] })
			}
			if err := rpc("play", Args{Tracks: ts}, nil, true); err != nil {
				return err
			}
			return printNow()
		},
	}
	c.Flags().StringVar(&album, "album", "", "id альбома")
	c.Flags().StringVar(&playlist, "playlist", "", "id плейлиста")
	c.Flags().StringVar(&artist, "artist", "", "id исполнителя (его топ-треки)")
	c.Flags().BoolVarP(&list, "list", "l", false, "очередь из результатов поиска вместо радио")
	c.Flags().BoolVarP(&shuffle, "shuffle", "s", false, "перемешать")
	return c
}

func cmdAdd() *cobra.Command {
	var next bool
	c := &cobra.Command{Use: "add <запрос | videoId>", Short: "Добавить трек в очередь", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			t, err := trackFrom(NewAPI(), strings.Join(args, " "))
			if err != nil {
				return err
			}
			if err := rpc("add", Args{Tracks: []Track{*t}, Next: next}, nil, true); err != nil {
				return err
			}
			fmt.Println("+", t.Label())
			return nil
		}}
	c.Flags().BoolVarP(&next, "next", "n", false, "сыграть следующим")
	return c
}

func cmdWave() *cobra.Command {
	var setOnly bool
	c := &cobra.Command{Use: "wave [mix|favorites|discover]", Short: "Моя волна (рекомендации по вашим сигналам)",
		Long: "mix — любимое вперемешку с новым, favorites — больше того, что любите, discover — только новое.\nС --mode меняет режим идущей волны без перезапуска.",
		Args: cobra.MaximumNArgs(1), ValidArgs: []string{"mix", "favorites", "discover"},
		RunE: func(_ *cobra.Command, args []string) error {
			mode := ""
			if len(args) > 0 {
				mode = args[0]
			}
			if setOnly {
				return rpc("wavemode", Args{Value: mode}, nil, true)
			}
			if err := rpc("wave", Args{Value: mode}, nil, true); err != nil {
				return err
			}
			return printNow()
		}}
	c.Flags().BoolVarP(&setOnly, "mode", "m", false, "только сменить режим")
	return c
}

func cmdSearch() *cobra.Command {
	var typ string
	var n int
	var asJSON bool
	c := &cobra.Command{Use: "search <запрос>", Short: "Поиск", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			filter := map[string]string{"song": "songs", "album": "albums", "artist": "artists", "playlist": "playlists", "video": "videos"}[strings.TrimSuffix(typ, "s")]
			if filter == "" && typ != "all" {
				return fmt.Errorf("тип: songs|albums|artists|playlists|videos|all")
			}
			res, err := NewAPI().Search(strings.Join(args, " "), filter, n)
			if err != nil {
				return err
			}
			if len(res) > n {
				res = res[:n]
			}
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(res)
			}
			for i, r := range res {
				id := r.VideoID
				if id == "" {
					id = r.BrowseID
				}
				title, sub := resultText(r)
				fmt.Printf("%3d  %-50s  %-30s  %s\n", i+1, clip(title, 50), clip(sub, 30), id)
			}
			return nil
		}}
	c.Flags().StringVarP(&typ, "type", "t", "songs", "songs|albums|artists|playlists|videos|all")
	c.Flags().IntVarP(&n, "limit", "n", 20, "сколько результатов")
	c.Flags().BoolVar(&asJSON, "json", false, "вывод в JSON")
	return c
}

func resultText(r SearchResult) (string, string) {
	switch r.ResultType {
	case "artist":
		name := r.ArtistName
		if name == "" {
			name = r.Title
		}
		return name, "исполнитель"
	case "album":
		return r.Title, strings.TrimSpace(r.Type + " · " + r.ArtistNames() + " " + r.Year)
	case "playlist":
		return r.Title, "плейлист · " + r.Author
	}
	sub := r.ArtistNames()
	if r.Duration != "" {
		sub += " · " + r.Duration
	}
	return r.Title, sub
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// ---------- control ----------

func cmdSeek() *cobra.Command {
	return &cobra.Command{Use: "seek <+10 | -10 | 1:23>", Short: "Перемотка (относительно или на позицию)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error { return rpc("seek", Args{Value: args[0]}, nil, false) }}
}

func cmdVolume() *cobra.Command {
	return &cobra.Command{Use: "vol [N | +5 | -5]", Aliases: []string{"volume"}, Short: "Громкость", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 1 {
				if err := rpc("volume", Args{Value: args[0]}, nil, true); err != nil {
					return err
				}
			}
			s, err := getStatus()
			if err != nil {
				return err
			}
			fmt.Printf("%.0f%%\n", s.Volume)
			return nil
		}}
}

func cmdRepeat() *cobra.Command {
	return &cobra.Command{Use: "repeat [off|all|one]", Short: "Повтор (без аргумента — по кругу)", Args: cobra.MaximumNArgs(1),
		ValidArgs: []string{"off", "all", "one"},
		RunE: func(_ *cobra.Command, args []string) error {
			v := ""
			if len(args) > 0 {
				v = args[0]
			}
			if err := rpc("repeat", Args{Value: v}, nil, true); err != nil {
				return err
			}
			s, _ := getStatus()
			fmt.Println("repeat:", s.Repeat)
			return nil
		}}
}

func fmtTime(s float64) string {
	if s < 0 || s != s {
		s = 0
	}
	return fmt.Sprintf("%d:%02d", int(s)/60, int(s)%60)
}

func cmdStatus() *cobra.Command {
	var format string
	var asJSON bool
	c := &cobra.Command{Use: "status", Short: "Что играет",
		Long: "Плейсхолдеры для -f: {title} {artist} {album} {pos} {dur} {state} {icon} {vol} {wave} {liked}\nНапример для waybar: bmusic status -f '{icon} {artist} — {title}'",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			s, err := getStatus()
			if errors.Is(err, errNoDaemon) {
				if format != "" || asJSON {
					return nil // quiet for status bars
				}
				return err
			}
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(s)
			}
			if s.Track == nil {
				if format == "" {
					fmt.Println("очередь пуста")
				}
				return nil
			}
			state, icon := "playing", "▶"
			if s.Paused {
				state, icon = "paused", "⏸"
			}
			album := ""
			if s.Track.Album != nil {
				album = s.Track.Album.Name
			}
			liked := ""
			if s.Liked {
				liked = "♥"
			}
			if format != "" {
				fmt.Println(strings.NewReplacer(
					"{title}", s.Track.Title, "{artist}", s.Track.ArtistNames(), "{album}", album,
					"{pos}", fmtTime(s.Pos), "{dur}", fmtTime(s.Dur), "{state}", state, "{icon}", icon,
					"{vol}", fmt.Sprintf("%.0f", s.Volume), "{wave}", waveModeName(s.Wave), "{liked}", liked,
				).Replace(format))
				return nil
			}
			fmt.Println(strings.TrimSpace(icon + " " + s.Track.Label() + " " + liked))
			if album != "" {
				fmt.Println("  альбом:", album)
			}
			fmt.Printf("  %s / %s   трек %d из %d   громкость %.0f%%\n", fmtTime(s.Pos), fmtTime(s.Dur), s.Index+1, s.Length, s.Volume)
			var flags []string
			if s.Wave != "" {
				flags = append(flags, "волна: "+waveModeName(s.Wave))
			}
			if s.Repeat != "off" {
				flags = append(flags, "повтор: "+s.Repeat)
			}
			if s.Shuffle {
				flags = append(flags, "перемешивание")
			}
			if s.Track.Reason != "" {
				flags = append(flags, s.Track.Reason)
			}
			if len(flags) > 0 {
				fmt.Println("  " + strings.Join(flags, " · "))
			}
			return nil
		}}
	c.Flags().StringVarP(&format, "format", "f", "", "формат строки")
	c.Flags().BoolVar(&asJSON, "json", false, "вывод в JSON")
	return c
}

func cmdQueue() *cobra.Command {
	var all bool
	c := &cobra.Command{Use: "queue", Short: "Очередь", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		q, err := getQueue()
		if err != nil {
			return err
		}
		lo, hi := 0, len(q.Queue)
		if !all {
			lo, hi = max(0, q.Idx-3), min(len(q.Queue), q.Idx+16)
		}
		for i := lo; i < hi; i++ {
			mark := "  "
			if i == q.Idx {
				mark = "▶ "
			}
			t := q.Queue[i]
			fmt.Printf("%s%3d  %-60s %s\n", mark, i+1, clip(t.Label(), 60), t.Duration)
		}
		if hi < len(q.Queue) {
			fmt.Printf("     … ещё %d\n", len(q.Queue)-hi)
		}
		return nil
	}}
	c.Flags().BoolVarP(&all, "all", "a", false, "вся очередь")
	c.AddCommand(
		&cobra.Command{Use: "clear", Short: "Очистить всё, кроме текущего", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			return rpc("clear", nil, nil, false)
		}},
		&cobra.Command{Use: "rm <N>", Short: "Убрать трек N из очереди", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return err
			}
			return rpc("remove", Args{Index: n - 1}, nil, false)
		}},
	)
	return c
}

func cmdJump() *cobra.Command {
	return &cobra.Command{Use: "jump <N>", Short: "Перейти к треку N очереди", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return err
			}
			if err := rpc("jump", Args{Index: n - 1}, nil, true); err != nil {
				return err
			}
			return printNow()
		}}
}

func cmdLike() *cobra.Command {
	return &cobra.Command{Use: "like", Short: "♥ текущий трек (повторно — убрать)", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			var liked bool
			if err := rpc("like", nil, &liked, false); err != nil {
				return err
			}
			if liked {
				fmt.Println("♥ в любимом")
			} else {
				fmt.Println("убрано из любимого")
			}
			return nil
		}}
}

// ---------- library ----------

func printTracks(ts []Track) {
	for i, t := range ts {
		fmt.Printf("%4d  %-60s %6s  %s\n", i+1, clip(t.Label(), 60), t.Duration, t.VideoID)
	}
}

func playTracks(ts []Track, shuffle bool) error {
	if shuffle {
		rand.Shuffle(len(ts), func(i, j int) { ts[i], ts[j] = ts[j], ts[i] })
	}
	if err := rpc("play", Args{Tracks: ts}, nil, true); err != nil {
		return err
	}
	return printNow()
}

func cmdLibrary(use, short string, load func(*API) ([]Track, error)) *cobra.Command {
	var play, shuffle bool
	c := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		ts, err := load(NewAPI())
		if err != nil {
			return err
		}
		if play || shuffle {
			return playTracks(ts, shuffle)
		}
		printTracks(ts)
		return nil
	}}
	c.Flags().BoolVarP(&play, "play", "p", false, "играть")
	c.Flags().BoolVarP(&shuffle, "shuffle", "s", false, "играть вперемешку")
	return c
}

func findAlbum(a *API, ref string) (*MyAlbum, error) {
	list, err := a.MyAlbums("")
	if err != nil {
		return nil, err
	}
	if n, err := strconv.Atoi(ref); err == nil {
		for _, x := range list {
			if x.ID == n {
				return &x, nil
			}
		}
	}
	var hits []MyAlbum
	for _, x := range list {
		if strings.EqualFold(x.Name, ref) {
			return &x, nil
		}
		if strings.Contains(strings.ToLower(x.Name), strings.ToLower(ref)) {
			hits = append(hits, x)
		}
	}
	if len(hits) == 1 {
		return &hits[0], nil
	}
	if len(hits) > 1 {
		return nil, fmt.Errorf("неоднозначно %q: подходят %d альбомов", ref, len(hits))
	}
	return nil, fmt.Errorf("нет альбома %q", ref)
}

func currentTrack() (*Track, error) {
	s, err := getStatus()
	if err != nil {
		return nil, err
	}
	if s.Track == nil {
		return nil, errors.New("ничего не играет")
	}
	return s.Track, nil
}

func cmdAlbums() *cobra.Command {
	c := &cobra.Command{Use: "albums", Aliases: []string{"album"}, Short: "Мои альбомы", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			list, err := NewAPI().MyAlbums("")
			if err != nil {
				return err
			}
			for _, x := range list {
				fmt.Printf("%4d  %-40s %d тр.\n", x.ID, x.Name, x.Count)
			}
			return nil
		}}
	var shuffle bool
	play := &cobra.Command{Use: "play <имя|id>", Short: "Играть альбом", Args: cobra.MinimumNArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		a := NewAPI()
		al, err := findAlbum(a, strings.Join(args, " "))
		if err != nil {
			return err
		}
		full, err := a.MyAlbum(al.ID)
		if err != nil {
			return err
		}
		return playTracks(full.Tracks, shuffle)
	}}
	play.Flags().BoolVarP(&shuffle, "shuffle", "s", false, "вперемешку")
	c.AddCommand(
		play,
		&cobra.Command{Use: "show <имя|id>", Short: "Треки альбома", Args: cobra.MinimumNArgs(1), RunE: func(_ *cobra.Command, args []string) error {
			a := NewAPI()
			al, err := findAlbum(a, strings.Join(args, " "))
			if err != nil {
				return err
			}
			full, err := a.MyAlbum(al.ID)
			if err != nil {
				return err
			}
			printTracks(full.Tracks)
			return nil
		}},
		&cobra.Command{Use: "new <имя>", Short: "Создать альбом", Args: cobra.MinimumNArgs(1), RunE: func(_ *cobra.Command, args []string) error {
			id, err := NewAPI().CreateAlbum(strings.Join(args, " "))
			if err == nil {
				fmt.Println("создан, id", id)
			}
			return err
		}},
		&cobra.Command{Use: "add <имя|id>", Short: "Добавить текущий трек в альбом", Args: cobra.MinimumNArgs(1), RunE: func(_ *cobra.Command, args []string) error {
			a := NewAPI()
			t, err := currentTrack()
			if err != nil {
				return err
			}
			al, err := findAlbum(a, strings.Join(args, " "))
			if err != nil {
				return err
			}
			if err := a.AlbumAdd(al.ID, *t); err != nil {
				return err
			}
			fmt.Printf("%s → %s\n", t.Label(), al.Name)
			return nil
		}},
		&cobra.Command{Use: "unadd <имя|id>", Short: "Убрать текущий трек из альбома", Args: cobra.MinimumNArgs(1), RunE: func(_ *cobra.Command, args []string) error {
			a := NewAPI()
			t, err := currentTrack()
			if err != nil {
				return err
			}
			al, err := findAlbum(a, strings.Join(args, " "))
			if err != nil {
				return err
			}
			return a.AlbumRemove(al.ID, t.VideoID)
		}},
		&cobra.Command{Use: "delete <имя|id>", Short: "Удалить альбом", Args: cobra.MinimumNArgs(1), RunE: func(_ *cobra.Command, args []string) error {
			a := NewAPI()
			al, err := findAlbum(a, strings.Join(args, " "))
			if err != nil {
				return err
			}
			return a.DeleteAlbum(al.ID)
		}},
	)
	return c
}

func cmdLyrics() *cobra.Command {
	return &cobra.Command{Use: "lyrics [videoId]", Short: "Текст песни (по умолчанию — текущей)", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = args[0]
			} else {
				t, err := currentTrack()
				if err != nil {
					return err
				}
				id = t.VideoID
				fmt.Printf("%s\n\n", t.Label())
			}
			text, src, err := NewAPI().Lyrics(id)
			if err != nil {
				return err
			}
			if text == "" {
				return errors.New("текста нет")
			}
			fmt.Println(text)
			if src != "" {
				fmt.Printf("\n%s\n", src)
			}
			return nil
		}}
}

func cmdDownload() *cobra.Command {
	var dir string
	c := &cobra.Command{Use: "download [запрос | videoId]", Short: "Скачать .m4a с тегами (по умолчанию — текущий трек)",
		RunE: func(_ *cobra.Command, args []string) error {
			a := NewAPI()
			var t *Track
			var err error
			if len(args) > 0 {
				t, err = trackFrom(a, strings.Join(args, " "))
			} else {
				t, err = currentTrack()
			}
			if err != nil {
				return err
			}
			fmt.Println("качаю", t.Label(), "…")
			path, err := a.Download(*t, dir)
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		}}
	c.Flags().StringVarP(&dir, "out", "o", ".", "куда сохранить")
	return c
}
