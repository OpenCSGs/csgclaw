package browser

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
)

func TestBrowserHistoryControlsRemainActive(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/details" {
			fmt.Fprint(w, `<title>Details</title><h1>Details</h1>`)
			return
		}
		fmt.Fprint(w, `<title>Form</title><h1>Form</h1><input aria-label="Editable" value="old"><a href="/details">Details link</a><div style="height:2500px"></div><button onclick="document.title='Footer clicked'">Footer</button>`)
	}))
	defer site.Close()
	service := New()
	defer service.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	state, err := service.Open(ctx, "history", site.URL)
	if err != nil {
		t.Fatal(err)
	}
	getRef := func(role, name string) string {
		t.Helper()
		snapshot, err := service.Snapshot(ctx, "history", state.ID)
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`\[(e\d+)\] ` + role + ` "` + name + `"`).FindStringSubmatch(snapshot.Content)
		if len(match) != 2 {
			t.Fatalf("missing active %s %s: %+v", role, name, snapshot)
		}
		return match[1]
	}
	if _, err = service.Run(ctx, "history", state.ID, Command{Action: "click", Ref: getRef("link", "Details link")}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		state, err = service.Run(ctx, "history", state.ID, Command{Action: "back"})
		if err != nil {
			t.Fatal(err)
		}
		if state.URL != site.URL+"/" {
			t.Fatalf("back returned the previous page URL: %s", state.URL)
		}
		snapshot, err := service.Snapshot(ctx, "history", state.ID)
		if err != nil || !strings.Contains(snapshot.Content, `heading "Form"`) {
			t.Fatalf("back exposed an inactive document: %v %+v", err, snapshot)
		}
		if _, err = service.Run(ctx, "history", state.ID, Command{Action: "fill", Ref: getRef("textbox", "Editable"), Text: "round-trip"}); err != nil {
			t.Fatalf("input after back: %v", err)
		}
		if _, err = service.Run(ctx, "history", state.ID, Command{Action: "forward"}); err != nil {
			t.Fatal(err)
		}
		if _, err = service.Run(ctx, "history", state.ID, Command{Action: "reload"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = service.Run(ctx, "history", state.ID, Command{Action: "back"}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Run(ctx, "history", state.ID, Command{Action: "scroll", Y: 2000}); err != nil {
		t.Fatal(err)
	}
	for {
		snapshot, err := service.Snapshot(ctx, "history", state.ID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.ScrollY > 0 && snapshot.ContentHeight > float64(snapshot.Height) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("scroll position was not exposed")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if _, err = service.Run(ctx, "history", state.ID, Command{Action: "click", Ref: getRef("button", "Footer")}); err != nil {
		t.Fatalf("click after history navigation and scrolling: %v", err)
	}
	for _, fragment := range []string{"#one", "#two"} {
		if _, err = service.Run(ctx, "history", state.ID, Command{Action: "navigate", URL: site.URL + "/" + fragment}); err != nil {
			t.Fatal(err)
		}
	}
	state, err = service.Run(ctx, "history", state.ID, Command{Action: "back"})
	if err != nil || !strings.HasSuffix(state.URL, "#one") {
		t.Fatalf("same-document back: %+v %v", state, err)
	}
	state, err = service.Run(ctx, "history", state.ID, Command{Action: "forward"})
	if err != nil || !strings.HasSuffix(state.URL, "#two") {
		t.Fatalf("same-document forward: %+v %v", state, err)
	}
}
