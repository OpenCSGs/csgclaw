package boxlitecli

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

var boxliteHomeLocks sync.Map

type boxliteHomeLock struct {
	ch chan struct{}
}

func acquireBoxliteHomeLock(ctx context.Context, homeDir string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := filepath.Clean(strings.TrimSpace(homeDir))
	lockValue, _ := boxliteHomeLocks.LoadOrStore(key, &boxliteHomeLock{ch: make(chan struct{}, 1)})
	lock := lockValue.(*boxliteHomeLock)

	select {
	case lock.ch <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-lock.ch
			return nil, err
		}
		return func() {
			<-lock.ch
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
