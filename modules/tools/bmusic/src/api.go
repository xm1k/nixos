package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ---------- types ----------

type Artist struct {
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
}

type AlbumRef struct {
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
}

type Thumb struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Track is the normalized track the music service stores (likes, history, albums)
// and the player queues. UnmarshalJSON accepts any ytmusicapi shape.
type Track struct {
	VideoID    string    `json:"videoId"`
	Title      string    `json:"title"`
	Artists    []Artist  `json:"artists"`
	Album      *AlbumRef `json:"album"`
	Duration   string    `json:"duration,omitempty"`
	Thumbnails []Thumb   `json:"thumbnails"`
	VideoType  string    `json:"videoType,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

func (t *Track) UnmarshalJSON(b []byte) error {
	var r struct {
		VideoID    string          `json:"videoId"`
		Title      string          `json:"title"`
		Artists    []Artist        `json:"artists"`
		Album      json.RawMessage `json:"album"`
		Duration   string          `json:"duration"`
		Length     string          `json:"length"`
		Thumbnails []Thumb         `json:"thumbnails"`
		Thumbnail  []Thumb         `json:"thumbnail"`
		VideoType  string          `json:"videoType"`
		Reason     string          `json:"reason"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	*t = Track{VideoID: r.VideoID, Title: r.Title, Duration: r.Duration, VideoType: r.VideoType, Reason: r.Reason}
	for _, a := range r.Artists {
		if a.Name != "" {
			t.Artists = append(t.Artists, a)
		}
	}
	if t.Duration == "" {
		t.Duration = r.Length
	}
	th := r.Thumbnails
	if len(th) == 0 {
		th = r.Thumbnail
	}
	if len(th) > 2 {
		th = th[len(th)-2:]
	}
	t.Thumbnails = th
	if len(r.Album) > 0 && string(r.Album) != "null" {
		var s string
		if json.Unmarshal(r.Album, &s) == nil {
			if s != "" {
				t.Album = &AlbumRef{Name: s}
			}
		} else {
			var a AlbumRef
			if json.Unmarshal(r.Album, &a) == nil && a.Name != "" {
				t.Album = &a
			}
		}
	}
	return nil
}

func (t Track) ArtistNames() string {
	n := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		n = append(n, a.Name)
	}
	return strings.Join(n, ", ")
}

func (t Track) Label() string {
	if a := t.ArtistNames(); a != "" {
		return a + " — " + t.Title
	}
	return t.Title
}

func (t Track) Seconds() float64 {
	var s float64
	for _, p := range strings.Split(t.Duration, ":") {
		var x float64
		fmt.Sscan(p, &x)
		s = s*60 + x
	}
	return s
}

// SearchResult is one row of /api/search (any resultType).
type SearchResult struct {
	Track
	ResultType string `json:"resultType"`
	Category   string `json:"category"`
	BrowseID   string `json:"browseId"`
	PlaylistID string `json:"playlistId"`
	ArtistName string `json:"artist"`
	Author     string `json:"author"`
	Year       string `json:"year"`
	Type       string `json:"type"`
	ItemCount  string `json:"itemCount"`
}

func (s *SearchResult) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &s.Track); err != nil {
		return err
	}
	var r struct {
		ResultType string          `json:"resultType"`
		Category   string          `json:"category"`
		BrowseID   string          `json:"browseId"`
		PlaylistID string          `json:"playlistId"`
		Artist     string          `json:"artist"`
		Author     json.RawMessage `json:"author"`
		Year       any             `json:"year"`
		Type       string          `json:"type"`
		ItemCount  any             `json:"itemCount"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	s.ResultType, s.Category, s.BrowseID, s.PlaylistID, s.ArtistName, s.Type =
		r.ResultType, r.Category, r.BrowseID, r.PlaylistID, r.Artist, r.Type
	if r.Year != nil {
		s.Year = fmt.Sprint(r.Year)
	}
	if r.ItemCount != nil {
		s.ItemCount = fmt.Sprint(r.ItemCount)
	}
	// author is a string in search results and a list of {name,id} elsewhere
	var as string
	if json.Unmarshal(r.Author, &as) == nil {
		s.Author = as
	} else {
		var al []Artist
		if json.Unmarshal(r.Author, &al) == nil && len(al) > 0 {
			s.Author = al[0].Name
		}
	}
	return nil
}

type Album struct {
	Title           string   `json:"title"`
	Type            string   `json:"type"`
	Year            string   `json:"year"`
	Artists         []Artist `json:"artists"`
	Thumbnails      []Thumb  `json:"thumbnails"`
	AudioPlaylistID string   `json:"audioPlaylistId"`
	Tracks          []Track  `json:"tracks"`
}

type ArtistPage struct {
	Name  string `json:"name"`
	Songs struct {
		BrowseID string  `json:"browseId"`
		Results  []Track `json:"results"`
	} `json:"songs"`
	Albums struct {
		Results []SearchResult `json:"results"`
	} `json:"albums"`
	Singles struct {
		Results []SearchResult `json:"results"`
	} `json:"singles"`
}

type Playlist struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Tracks []Track `json:"tracks"`
}

type MyAlbum struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Count  int     `json:"count"`
	Has    bool    `json:"has"`
	Tracks []Track `json:"tracks,omitempty"`
}

type WaveResp struct {
	Tracks []Track `json:"tracks"`
	Seeds  []struct {
		Title  string `json:"title"`
		Origin string `json:"origin"`
	} `json:"seeds"`
}

type Taste struct {
	Artists []struct {
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	} `json:"artists"`
	Likes int     `json:"likes"`
	Hours float64 `json:"hours"`
	Ready bool    `json:"ready"`
}

type HomeShelf struct {
	Title    string         `json:"title"`
	Contents []SearchResult `json:"contents"`
}

var WaveModes = []struct{ ID, Name, Desc string }{
	{"mix", "Микс", "Любимое вперемешку с новым"},
	{"favorites", "Любимое", "Больше того, что вы уже любите"},
	{"discover", "Открытия", "Только то, что вы ещё не слышали"},
}

func waveModeName(id string) string {
	for _, m := range WaveModes {
		if m.ID == id {
			return m.Name
		}
	}
	return id
}

// ---------- client ----------

var ErrAuth = errors.New("нужно войти: bmusic login")

// b.net is reachable only directly (tailscale): never go through the corporate proxy.
var directTransport = &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 8, IdleConnTimeout: 90 * time.Second}

type API struct {
	Base  string
	Token string
	hc    *http.Client
}

func NewAPI() *API {
	return &API{Base: musicURL(), Token: loadToken(), hc: &http.Client{Transport: directTransport, Timeout: 60 * time.Second}}
}

func musicURL() string {
	if u := os.Getenv("BMUSIC_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://music.b.net"
}

func authURL() string {
	if u := os.Getenv("BMUSIC_AUTH_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://b.net"
}

func (a *API) do(method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.Base+"/api/"+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.Token != "" {
		req.AddCookie(&http.Cookie{Name: "bb_sess", Value: a.Token})
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return ErrAuth
	}
	if resp.StatusCode == 403 {
		return errors.New("нет доступа к music.b.net для этого пользователя")
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (a *API) get(path string, out any) error { return a.do("GET", path, nil, out) }

func (a *API) Me() (string, error) {
	var r struct{ User string }
	err := a.get("me", &r)
	return r.User, err
}

func (a *API) Search(q, filter string, limit int) ([]SearchResult, error) {
	v := url.Values{"q": {q}, "limit": {fmt.Sprint(limit)}}
	if filter != "" {
		v.Set("filter", filter)
	}
	var r []SearchResult
	err := a.get("search?"+v.Encode(), &r)
	return r, err
}

func (a *API) SearchSongs(q string, limit int) ([]Track, error) {
	res, err := a.Search(q, "songs", limit)
	var out []Track
	for _, r := range res {
		if r.VideoID != "" {
			out = append(out, r.Track)
		}
	}
	return out, err
}

func (a *API) Album(id string) (*Album, error) {
	var r Album
	if err := a.get("album/"+url.PathEscape(id), &r); err != nil {
		return nil, err
	}
	// album tracks come without their own covers
	for i := range r.Tracks {
		if len(r.Tracks[i].Thumbnails) == 0 {
			r.Tracks[i].Thumbnails = lastThumbs(r.Thumbnails)
		}
		if r.Tracks[i].Album == nil || r.Tracks[i].Album.ID == "" {
			r.Tracks[i].Album = &AlbumRef{Name: r.Title, ID: id}
		}
	}
	return &r, nil
}

func lastThumbs(t []Thumb) []Thumb {
	if len(t) > 2 {
		return t[len(t)-2:]
	}
	return t
}

func (a *API) Artist(id string) (*ArtistPage, error) {
	var r ArtistPage
	err := a.get("artist/"+url.PathEscape(id), &r)
	return &r, err
}

func (a *API) Playlist(id string) (*Playlist, error) {
	var r Playlist
	err := a.get("playlist/"+url.PathEscape(id), &r)
	return &r, err
}

func (a *API) Radio(videoID string) ([]Track, error) {
	var r struct{ Tracks []Track }
	err := a.get("radio/"+url.PathEscape(videoID), &r)
	return validTracks(r.Tracks), err
}

func (a *API) PlaylistRadio(playlistID string) ([]Track, error) {
	var r struct{ Tracks []Track }
	err := a.get("radio/x?playlist_id="+url.QueryEscape(playlistID), &r)
	return validTracks(r.Tracks), err
}

func validTracks(ts []Track) []Track {
	out := ts[:0]
	for _, t := range ts {
		if t.VideoID != "" {
			out = append(out, t)
		}
	}
	return out
}

func (a *API) Lyrics(videoID string) (string, string, error) {
	var r struct {
		Lyrics any    `json:"lyrics"`
		Source string `json:"source"`
	}
	if err := a.get("lyrics/"+url.PathEscape(videoID), &r); err != nil {
		return "", "", err
	}
	if s, ok := r.Lyrics.(string); ok {
		return s, r.Source, nil
	}
	return "", "", nil
}

func (a *API) Home() ([]HomeShelf, error) {
	var r []HomeShelf
	err := a.get("home?limit=8", &r)
	return r, err
}

func (a *API) Wave(mode string, exclude []string) (*WaveResp, error) {
	v := url.Values{"mode": {mode}}
	if len(exclude) > 0 {
		v.Set("exclude", strings.Join(exclude, ","))
	}
	var r WaveResp
	err := a.get("wave?"+v.Encode(), &r)
	return &r, err
}

func (a *API) Taste() (*Taste, error) {
	var r Taste
	err := a.get("wave/taste", &r)
	return &r, err
}

func (a *API) Likes() ([]Track, error) {
	var r []Track
	err := a.get("likes", &r)
	return r, err
}

func (a *API) History() ([]Track, error) {
	var r []Track
	err := a.get("history", &r)
	return r, err
}

func (a *API) Dislikes() ([]Track, error) {
	var r []Track
	err := a.get("dislikes", &r)
	return r, err
}

func (a *API) Like(t Track) error       { return a.do("PUT", "likes", t, nil) }
func (a *API) Unlike(id string) error   { return a.do("DELETE", "likes/"+url.PathEscape(id), nil, nil) }
func (a *API) Dislike(t Track) error    { return a.do("PUT", "dislikes", t, nil) }
func (a *API) AddHistory(t Track) error { return a.do("POST", "history", t, nil) }
func (a *API) Prepare(id string) error  { return a.do("POST", "prepare/"+url.PathEscape(id), nil, nil) }

func (a *API) Event(t Track, kind string, listened float64) error {
	return a.do("POST", "events", map[string]any{"track": t, "kind": kind, "listened": int(listened + 0.5)}, nil)
}

func (a *API) MyAlbums(videoID string) ([]MyAlbum, error) {
	var r []MyAlbum
	err := a.get("my-albums?video_id="+url.QueryEscape(videoID), &r)
	return r, err
}

func (a *API) MyAlbum(id int) (*MyAlbum, error) {
	var r MyAlbum
	err := a.get(fmt.Sprintf("my-albums/%d", id), &r)
	return &r, err
}

func (a *API) CreateAlbum(name string) (int, error) {
	var r struct{ ID int }
	err := a.do("POST", "my-albums", map[string]string{"name": name}, &r)
	return r.ID, err
}

func (a *API) RenameAlbum(id int, name string) error {
	return a.do("PATCH", fmt.Sprintf("my-albums/%d", id), map[string]string{"name": name}, nil)
}

func (a *API) DeleteAlbum(id int) error {
	return a.do("DELETE", fmt.Sprintf("my-albums/%d", id), nil, nil)
}

func (a *API) AlbumAdd(id int, t Track) error {
	return a.do("PUT", fmt.Sprintf("my-albums/%d/tracks", id), t, nil)
}

func (a *API) AlbumRemove(id int, videoID string) error {
	return a.do("DELETE", fmt.Sprintf("my-albums/%d/tracks/%s", id, url.PathEscape(videoID)), nil, nil)
}

func (a *API) StreamURL(id string) string { return a.Base + "/api/stream/" + url.PathEscape(id) }

// Download saves the tagged .m4a the server builds for a track.
func (a *API) Download(t Track, dir string) (string, error) {
	v := url.Values{"title": {t.Title}, "artist": {t.ArtistNames()}}
	if t.Album != nil {
		v.Set("album", t.Album.Name)
	}
	if len(t.Thumbnails) > 0 {
		v.Set("cover", bigThumb(t.Thumbnails[len(t.Thumbnails)-1].URL))
	}
	req, _ := http.NewRequest("GET", a.Base+"/api/download/"+url.PathEscape(t.VideoID)+"?"+v.Encode(), nil)
	req.AddCookie(&http.Cookie{Name: "bb_sess", Value: a.Token})
	hc := &http.Client{Transport: directTransport, Timeout: 10 * time.Minute}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download: %d", resp.StatusCode)
	}
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 32 {
			return ' '
		}
		return r
	}, t.Label())
	path := dir + "/" + strings.TrimSpace(name) + ".m4a"
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return path, err
}

func bigThumb(u string) string {
	if i := strings.Index(u, "=w"); i > 0 && (strings.Contains(u, "googleusercontent") || strings.Contains(u, "ggpht")) {
		return u[:i] + "=w544-h544-l90-rj"
	}
	return u
}

// Login exchanges b.net credentials for the bb_sess session cookie.
func Login(user, pass string) (string, error) {
	b, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	hc := &http.Client{Transport: directTransport, Timeout: 15 * time.Second}
	resp, err := hc.Post(authURL()+"/api/login", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return "", errors.New("неверный логин или пароль")
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("login: %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "bb_sess" {
			return c.Value, nil
		}
	}
	return "", errors.New("сервер не выдал сессию")
}
