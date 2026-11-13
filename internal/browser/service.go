// Package browser owns the Chromium sessions shared by Agents and the chat UI.
package browser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const Width, Height = 1280, 800

var ErrNotFound = errors.New("browser session not found in this room")

type State struct {
	ID           string `json:"id"`
	RoomID       string `json:"room_id"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Revision     uint64 `json:"revision"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	CanGoBack    bool   `json:"can_go_back"`
	CanGoForward bool   `json:"can_go_forward"`
}

type Command struct {
	Action   string  `json:"action"`
	URL      string  `json:"url,omitempty"`
	Ref      string  `json:"ref,omitempty"`
	Text     string  `json:"text,omitempty"`
	Key      string  `json:"key,omitempty"`
	X        float64 `json:"x,omitempty"`
	Y        float64 `json:"y,omitempty"`
	FullPage bool    `json:"full_page,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
}

type Snapshot struct {
	State
	Content       string  `json:"content"`
	ScrollX       int     `json:"scroll_x"`
	ScrollY       int     `json:"scroll_y"`
	ContentHeight float64 `json:"content_height"`
}

type session struct {
	gate     chan struct{}
	page     *rod.Page
	context  *rod.Browser
	state    State                             // guarded by Service.mu
	refs     map[string]proto.DOMBackendNodeID // guarded by gate
	lastUsed time.Time
}

type Service struct {
	mu          sync.Mutex
	launchMu    sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	browser     *rod.Browser
	launcher    *launcher.Launcher
	sessions    map[string]*session
	closedRooms map[string]bool
	closed      bool
	reapOnce    sync.Once
}

func New() *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{ctx: ctx, cancel: cancel, sessions: map[string]*session{}, closedRooms: map[string]bool{}}
	return s
}

// Chromium is launched only when the first page is requested, with a disposable
// profile. Each session has its own incognito context, never the user's profile.
func (s *Service) start(ctx context.Context) (*rod.Browser, error) {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	if s.ctx.Err() != nil {
		return nil, errors.New("browser service is closed")
	}
	s.reapOnce.Do(func() { go s.reap() })
	if s.browser != nil {
		if _, err := (proto.BrowserGetVersion{}).Call(s.browser.Context(ctx)); err == nil {
			return s.browser, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// A crashed Chromium cannot preserve its incognito sessions. Drop their
		// stale inventory and allow the next browser_open to recover.
		s.launcher.Kill()
		s.launcher.Cleanup()
		s.browser, s.launcher = nil, nil
		s.mu.Lock()
		clear(s.sessions)
		s.mu.Unlock()
	}
	bin := os.Getenv("CSGCLAW_BROWSER_BIN")
	if bin == "" {
		bin, _ = launcher.LookPath()
	}
	if bin == "" {
		return nil, errors.New("Chromium is unavailable; install Chrome, Edge or Chromium, or set CSGCLAW_BROWSER_BIN to its executable and restart CSGClaw")
	}
	// Keep navigation from freezing a controlled document in Chromium's
	// back/forward cache while its DevTools session still owns input events.
	l := launcher.New().Bin(bin).Headless(true).Set("disable-back-forward-cache")
	control, err := l.Context(ctx).Launch()
	if err != nil {
		l.Kill()
		l.Cleanup()
		return nil, fmt.Errorf("start Chromium: %w", err)
	}
	b := rod.New().ControlURL(control).Context(s.ctx).NoDefaultDevice()
	if err = b.Connect(); err != nil {
		l.Kill()
		l.Cleanup()
		return nil, fmt.Errorf("connect Chromium: %w", err)
	}
	s.launcher, s.browser = l, b
	return b, nil
}

func validURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", errors.New("a valid http or https URL without embedded credentials is required")
	}
	return u.String(), nil
}

func (s *Service) Open(ctx context.Context, roomID, rawURL string) (State, error) {
	if strings.TrimSpace(roomID) == "" {
		return State{}, errors.New("room_id is required")
	}
	u, err := validURL(rawURL)
	if err != nil {
		return State{}, err
	}
	b, err := s.start(ctx)
	if err != nil {
		return State{}, err
	}
	s.mu.Lock()
	if s.closed || s.closedRooms[roomID] {
		s.mu.Unlock()
		return State{}, errors.New("browser service is closed")
	}
	if len(s.sessions) >= 32 {
		s.mu.Unlock()
		return State{}, errors.New("browser session limit reached; close an existing browser first")
	}
	// Reserve a slot before creating Chromium resources.
	idBytes := make([]byte, 16)
	if _, err = rand.Read(idBytes); err != nil {
		s.mu.Unlock()
		return State{}, err
	}
	id := hex.EncodeToString(idBytes)
	e := &session{gate: make(chan struct{}, 1), state: State{ID: id, RoomID: roomID, URL: u, Width: Width, Height: Height}, lastUsed: time.Now()}
	e.gate <- struct{}{}
	s.sessions[id] = e
	s.mu.Unlock()
	incognito, err := b.Context(ctx).Incognito()
	if err == nil {
		e.context = incognito.Context(s.ctx)
		// Rod binds a page's event stream and input devices to its creation
		// context. Do not bind them to the short-lived MCP/HTTP request.
		e.page, err = e.context.Page(proto.TargetCreateTarget{})
	}
	if err == nil {
		err = e.page.Context(ctx).SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: Width, Height: Height, DeviceScaleFactor: 1})
	}
	if err == nil {
		err = e.page.Context(ctx).Navigate(u)
	}
	if err == nil {
		err = waitDocument(e.page.Context(ctx))
	}
	if err == nil {
		err = s.update(ctx, e)
	}
	s.mu.Lock()
	if s.closedRooms[roomID] {
		err = ErrNotFound
	}
	s.mu.Unlock()
	<-e.gate
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		_ = s.CloseSession(cleanupCtx, roomID, id)
		return State{}, err
	}
	s.mu.Lock()
	state := e.state
	s.mu.Unlock()
	return state, nil
}

func (s *Service) acquire(ctx context.Context, room, id string) (*session, func(), error) {
	return s.lockSession(ctx, room, id, true)
}

func (s *Service) lockSession(ctx context.Context, room, id string, touch bool) (*session, func(), error) {
	s.mu.Lock()
	e := s.sessions[id]
	if e == nil || e.state.RoomID != room || s.closed {
		s.mu.Unlock()
		return nil, nil, ErrNotFound
	}
	s.mu.Unlock()
	select {
	case e.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	s.mu.Lock()
	if s.sessions[id] != e {
		s.mu.Unlock()
		<-e.gate
		return nil, nil, ErrNotFound
	}
	if touch {
		e.lastUsed = time.Now()
	}
	s.mu.Unlock()
	return e, func() { <-e.gate }, nil
}

func (s *Service) List(room string) []State {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []State{}
	if s.closed || s.closedRooms[room] {
		return out
	}
	for _, e := range s.sessions {
		if e.state.RoomID == room && e.state.Revision > 0 {
			out = append(out, e.state)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) update(ctx context.Context, e *session) error {
	p := e.page.Context(ctx)
	info, err := p.Info()
	if err != nil {
		return fmt.Errorf("read browser page information: %w", err)
	}
	history, err := readDocument(ctx, p.GetNavigationHistory)
	if err != nil {
		return fmt.Errorf("read browser navigation history: %w", err)
	}
	s.mu.Lock()
	back, forward := history.CurrentIndex > 0, history.CurrentIndex+1 < len(history.Entries)
	if e.state.URL != info.URL || e.state.Title != info.Title || e.state.CanGoBack != back || e.state.CanGoForward != forward || e.state.Revision == 0 {
		e.state.URL, e.state.Title = info.URL, info.Title
		e.state.CanGoBack, e.state.CanGoForward = back, forward
		e.state.Revision++
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) Snapshot(ctx context.Context, room, id string) (Snapshot, error) {
	e, release, err := s.acquire(ctx, room, id)
	if err != nil {
		return Snapshot{}, err
	}
	defer release()
	if err = s.update(ctx, e); err != nil {
		return Snapshot{}, err
	}
	tree, err := readDocument(ctx, func() (*proto.AccessibilityGetFullAXTreeResult, error) {
		return (proto.AccessibilityGetFullAXTree{}).Call(e.page.Context(ctx))
	})
	if err != nil {
		return Snapshot{}, err
	}
	metrics, err := readDocument(ctx, func() (*proto.PageGetLayoutMetricsResult, error) {
		return (proto.PageGetLayoutMetrics{}).Call(e.page.Context(ctx))
	})
	if err != nil {
		return Snapshot{}, err
	}
	if metrics.CSSLayoutViewport == nil || metrics.CSSContentSize == nil {
		return Snapshot{}, errors.New("browser viewport metrics are unavailable")
	}
	refs := map[string]proto.DOMBackendNodeID{}
	var content strings.Builder
	for _, n := range tree.Nodes {
		if n.Ignored || n.Role == nil {
			continue
		}
		role := n.Role.Value.Str()
		if role == "InlineTextBox" {
			continue
		}
		name := ""
		if n.Name != nil {
			name = n.Name.Value.Str()
		}
		if role == "generic" && name == "" {
			continue
		}
		ref := ""
		if n.BackendDOMNodeID != 0 {
			ref = "e" + strconv.Itoa(int(n.BackendDOMNodeID))
			refs[ref] = n.BackendDOMNodeID
		}
		fmt.Fprintf(&content, "[%s] %s %s", ref, role, strconv.Quote(truncate(name, 2000)))
		if n.Value != nil {
			fmt.Fprintf(&content, " value=%s", strconv.Quote(truncate(n.Value.Value.Str(), 2000)))
		}
		for _, property := range n.Properties {
			switch property.Name {
			case proto.AccessibilityAXPropertyNameChecked, proto.AccessibilityAXPropertyNameSelected, proto.AccessibilityAXPropertyNameExpanded, proto.AccessibilityAXPropertyNameDisabled, proto.AccessibilityAXPropertyNameRequired:
				if property.Value != nil {
					fmt.Fprintf(&content, " %s=%s", property.Name, property.Value.Value.JSON("", ""))
				}
			}
		}
		content.WriteByte('\n')
		if content.Len() >= 48000 {
			content.WriteString("[snapshot truncated]\n")
			break
		}
	}
	e.refs = refs
	s.mu.Lock()
	state := e.state
	s.mu.Unlock()
	return Snapshot{State: state, Content: content.String(), ScrollX: metrics.CSSLayoutViewport.PageX, ScrollY: metrics.CSSLayoutViewport.PageY, ContentHeight: metrics.CSSContentSize.Height}, nil
}

func truncate(text string, max int) string {
	if len(text) <= max {
		return text
	}
	text = text[:max]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text
}

func (s *Service) Run(ctx context.Context, room, id string, c Command) (State, error) {
	e, release, err := s.acquire(ctx, room, id)
	if err != nil {
		return State{}, err
	}
	defer release()
	p := e.page.Context(ctx)
	var el *rod.Element
	if err = waitDocument(p); err != nil {
		return State{}, err
	}
	if c.Action == "click" || c.Action == "fill" || c.Action == "select" {
		node, ok := e.refs[c.Ref]
		if !ok {
			return State{}, errors.New("unknown element ref; call browser_snapshot first")
		}
		el, err = p.ElementFromNode(&proto.DOMNode{BackendNodeID: node})
		if err != nil {
			return State{}, errors.New("element is stale; call browser_snapshot again")
		}
	}
	switch c.Action {
	case "navigate":
		var u string
		u, err = validURL(c.URL)
		if err == nil {
			err = p.Navigate(u)
		}
		if err == nil {
			err = waitDocument(p)
		}
	case "back":
		err = navigateHistory(p, -1)
	case "forward":
		err = navigateHistory(p, 1)
	case "reload":
		err = p.Reload()
		if err == nil {
			err = waitDocument(p)
		}
	case "click":
		err = el.Click(proto.InputMouseButtonLeft, 1)
	case "fill":
		err = fillElement(el, c.Text)
	case "select":
		err = el.Select([]string{c.Text}, true, rod.SelectorTypeText)
	case "pointer":
		s.mu.Lock()
		width, height := e.state.Width, e.state.Height
		s.mu.Unlock()
		if c.X < 0 || c.Y < 0 || c.X >= float64(width) || c.Y >= float64(height) {
			return State{}, errors.New("pointer outside viewport")
		}
		err = p.Mouse.MoveTo(proto.Point{X: c.X, Y: c.Y})
		if err == nil {
			err = p.Mouse.Click(proto.InputMouseButtonLeft, 1)
		}
	case "type":
		err = p.InsertText(c.Text)
	case "key":
		keys := map[string]input.Key{"Enter": input.Enter, "Tab": input.Tab, "Escape": input.Escape, "Backspace": input.Backspace, "Delete": input.Delete, "ArrowDown": input.ArrowDown, "ArrowUp": input.ArrowUp, "ArrowLeft": input.ArrowLeft, "ArrowRight": input.ArrowRight, "Space": input.Space, "Home": input.Home, "End": input.End, "PageDown": input.PageDown, "PageUp": input.PageUp}
		key, ok := keys[c.Key]
		if !ok {
			return State{}, errors.New("unsupported browser key")
		}
		err = p.Keyboard.Type(key)
	case "scroll":
		if c.X < -10000 || c.X > 10000 || c.Y < -10000 || c.Y > 10000 {
			return State{}, errors.New("scroll distance exceeds limit")
		}
		err = p.Mouse.Scroll(c.X, c.Y, 1)
	case "resize":
		if c.Width < 320 || c.Width > 1920 || c.Height < 200 || c.Height > 1440 {
			return State{}, errors.New("viewport must be between 320x200 and 1920x1440")
		}
		err = p.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: c.Width, Height: c.Height, DeviceScaleFactor: 1})
		if err == nil {
			s.mu.Lock()
			e.state.Width, e.state.Height = c.Width, c.Height
			s.mu.Unlock()
		}
	default:
		return State{}, errors.New("unsupported browser action")
	}
	if err != nil {
		return State{}, err
	}
	if err = s.update(ctx, e); err != nil {
		return State{}, err
	}
	s.mu.Lock()
	state := e.state
	s.mu.Unlock()
	return state, nil
}

func (s *Service) Screenshot(ctx context.Context, room, id string, full bool) ([]byte, error) {
	e, release, err := s.acquire(ctx, room, id)
	if err != nil {
		return nil, err
	}
	defer release()
	if err = s.update(ctx, e); err != nil {
		return nil, err
	}
	return e.page.Context(ctx).Screenshot(full, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
}

func (s *Service) CloseSession(ctx context.Context, room, id string) error {
	return s.closeSession(ctx, room, id, time.Time{})
}

func (s *Service) closeSession(ctx context.Context, room, id string, idleBefore time.Time) error {
	e, release, err := s.lockSession(ctx, room, id, false)
	if err != nil {
		return err
	}
	defer release()
	s.mu.Lock()
	if !idleBefore.IsZero() && !e.lastUsed.Before(idleBefore) {
		s.mu.Unlock()
		return nil
	}
	delete(s.sessions, id)
	s.mu.Unlock()
	if e.context != nil {
		cleanupCtx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		return e.context.Context(cleanupCtx).Close()
	}
	return nil
}

func (s *Service) CloseRoom(ctx context.Context, room string) {
	s.mu.Lock()
	s.closedRooms[room] = true
	var sessions []State
	for _, e := range s.sessions {
		if e.state.RoomID == room {
			sessions = append(sessions, e.state)
		}
	}
	s.mu.Unlock()
	for _, state := range sessions {
		_ = s.CloseSession(ctx, room, state.ID)
	}
}

func (s *Service) reap() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			idleBefore := time.Now().Add(-30 * time.Minute)
			s.mu.Lock()
			var expired []State
			for _, e := range s.sessions {
				if e.lastUsed.Before(idleBefore) {
					expired = append(expired, e.state)
				}
			}
			s.mu.Unlock()
			for _, state := range expired {
				ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
				_ = s.closeSession(ctx, state.RoomID, state.ID, idleBefore)
				cancel()
			}
		}
	}
}

func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	if s.launcher != nil {
		s.launcher.Kill()
		s.launcher.Cleanup()
	}
}
