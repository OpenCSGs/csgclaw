//go:build windows

package browser

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/processgroup"
	"github.com/go-rod/rod/lib/launcher"
)

func TestMain(m *testing.M) {
	mode := os.Getenv("CSGCLAW_BROWSER_PROCESS_TEST")
	if mode == "leaf" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if mode == "ready" || mode == "waiting" || mode == "delegating" {
		child := exec.Command(os.Args[0], os.Args[1:]...)
		childMode := "leaf"
		if mode == "delegating" {
			childMode = "ready-delegated"
		}
		child.Env = append(os.Environ(), "CSGCLAW_BROWSER_PROCESS_TEST="+childMode)
		if err := child.Start(); err != nil {
			panic(err)
		}
		data, _ := json.Marshal([]int{os.Getpid(), child.Process.Pid})
		if err := os.WriteFile(os.Getenv("CSGCLAW_BROWSER_TEST_PIDS"), data, 0600); err != nil {
			panic(err)
		}
		if mode == "delegating" {
			os.Exit(0)
		}
	}
	if mode == "ready" || mode == "ready-delegated" || mode == "waiting" {
		if mode != "waiting" {
			for _, arg := range os.Args[1:] {
				if dir, ok := strings.CutPrefix(arg, "--user-data-dir="); ok {
					if err := os.WriteFile(filepath.Join(dir, "DevToolsActivePort"), []byte("12345\n/devtools/browser/fixture\n"), 0600); err != nil {
						panic(err)
					}
				}
			}
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	os.Exit(m.Run())
}

func TestWindowsBrowserOwnsDescendantsWithoutLeakless(t *testing.T) {
	for _, mode := range []string{"ready", "waiting", "delegating"} {
		t.Run(mode, func(t *testing.T) {
			pidsFile := filepath.Join(t.TempDir(), "pids.json")
			t.Setenv("CSGCLAW_BROWSER_PROCESS_TEST", mode)
			t.Setenv("CSGCLAW_BROWSER_TEST_PIDS", pidsFile)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type result struct {
				stop func()
				err  error
			}
			done := make(chan result, 1)
			go func() { _, stop, err := launchChromium(ctx, launcher.New().Bin(os.Args[0])); done <- result{stop, err} }()
			var pids []int
			deadline := time.After(20 * time.Second)
			for len(pids) == 0 {
				if raw, err := os.ReadFile(pidsFile); err == nil {
					_ = json.Unmarshal(raw, &pids)
				}
				select {
				case <-deadline:
					t.Fatal("browser helper did not start")
				default:
				}
				time.Sleep(10 * time.Millisecond)
			}
			if mode == "waiting" {
				cancel()
			}
			select {
			case r := <-done:
				if mode != "waiting" {
					if r.err != nil {
						t.Fatal(r.err)
					}
					r.stop()
					r.stop()
				} else if !errors.Is(r.err, context.Canceled) {
					t.Fatalf("canceled startup: %v", r.err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("browser shutdown did not complete")
			}
			for _, pid := range pids {
				if alive, err := processgroup.Alive(pid); err != nil || alive {
					t.Fatalf("browser descendant %d survived cleanup: alive=%v err=%v", pid, alive, err)
				}
			}
		})
	}
}
