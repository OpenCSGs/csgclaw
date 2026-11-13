package browser

import (
	"context"
	"github.com/go-rod/rod/lib/launcher"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestBrowserDateInputEndToEnd(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<label for="date">日期</label><input id="date" type="date"><output id="result">Date value: empty</output><script>document.querySelector('#date').addEventListener('input',event=>document.querySelector('#result').textContent='Date value: '+(event.target.value||'[empty]'))</script>`))
	}))
	defer page.Close()
	service := New()
	defer service.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	state, err := service.Open(ctx, "date-room", page.URL)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx, "date-room", state.ID)
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`\[(e\d+)\] Date "日期"`).FindStringSubmatch(snapshot.Content)
	if len(match) != 2 {
		t.Fatalf("date control not exposed: %s", snapshot.Content)
	}
	if _, err = service.Run(ctx, "date-room", state.ID, Command{Action: "fill", Ref: match[1], Text: "2026-11-20"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx, "date-room", state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot.Content, `"Date value: 2026-11-20"`) {
		t.Fatalf("date fill did not set the requested value: %s", snapshot.Content)
	}
	if _, err = service.Run(ctx, "date-room", state.ID, Command{Action: "fill", Ref: match[1], Text: "not-a-date"}); err == nil {
		t.Fatal("invalid date was accepted")
	}
	snapshot, err = service.Snapshot(ctx, "date-room", state.ID)
	if err != nil || !strings.Contains(snapshot.Content, `"Date value: 2026-11-20"`) {
		t.Fatalf("invalid fill destroyed the previous value: %v %s", err, snapshot.Content)
	}
	if _, err = service.Run(ctx, "date-room", state.ID, Command{Action: "fill", Ref: match[1], Text: ""}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx, "date-room", state.ID)
	if err != nil || !strings.Contains(snapshot.Content, `"Date value: [empty]"`) {
		t.Fatalf("date clearing did not notify the form: %v %s", err, snapshot.Content)
	}
}
