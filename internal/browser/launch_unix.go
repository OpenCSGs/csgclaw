//go:build !windows

package browser

import (
	"context"
	"os"
	"os/exec"
	"sync"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
)

func launchChromium(ctx context.Context, l *launcher.Launcher) (string, func(), error) {
	if _, err := exec.LookPath(l.Get(flags.Bin)); err != nil {
		return "", nil, err
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			// Rod never closes its exit channel when process creation fails.
			if l.PID() == 0 {
				_ = os.RemoveAll(l.Get("user-data-dir"))
				return
			}
			l.Kill()
			l.Cleanup()
		})
	}
	control, err := l.Context(ctx).Launch()
	if err != nil {
		stop()
		return "", nil, err
	}
	return control, stop, nil
}
