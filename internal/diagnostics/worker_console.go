package diagnostics

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const maxConsoleLine = 32 << 10

type consoleRecord struct {
	text    string
	flushed chan struct{}
}

// A stalled container log collector must not block the worker, log downloads
// or process teardown. Only the console copy may be dropped under pressure.
type consoleOutput struct {
	output  io.Writer
	once    sync.Once
	lines   chan consoleRecord
	dropped atomic.Uint64
}

var workerConsole = &consoleOutput{}

func (c *consoleOutput) start() {
	c.once.Do(func() {
		if c.output == nil {
			c.output = workerStdout()
		}
		c.lines = make(chan consoleRecord, 256)
		go func() {
			for record := range c.lines {
				if count := c.dropped.Swap(0); count != 0 {
					_, _ = fmt.Fprintf(c.output, "source=worker.stderr dropped_console_records=%d\n", count)
				}
				if record.flushed != nil {
					close(record.flushed)
					continue
				}
				_, _ = io.WriteString(c.output, record.text)
			}
		}()
	})
}

func (c *consoleOutput) offer(prefix string, line []byte) {
	c.start()
	select {
	case c.lines <- consoleRecord{text: fmt.Sprintf("%s msg=%q\n", prefix, line)}:
	default:
		c.dropped.Add(1)
	}
}

func (c *consoleOutput) flush() {
	c.start()
	done := make(chan struct{})
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case c.lines <- consoleRecord{flushed: done}:
	case <-timer.C:
		return
	}
	select {
	case <-done:
	case <-timer.C:
	}
}
