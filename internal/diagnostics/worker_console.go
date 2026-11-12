package diagnostics

import (
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
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

func (c *consoleOutput) offer(prefix, line string) {
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

// Match credential values even when a worker prints them without a field name.
func workerConsoleSecrets(env []string) *strings.Replacer {
	values := make(map[string]struct{})
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if value == "" || (!secretField(key) && !strings.EqualFold(key, "CSGCLAW_CONNECTOR_CAPABILITY")) {
			continue
		}
		quoted := strconv.Quote(value)
		values[quoted[1:len(quoted)-1]] = struct{}{}
		for line := range strings.Lines(value) {
			if line = strings.TrimRight(line, "\r\n"); line != "" {
				values[line] = struct{}{}
			}
		}
	}
	secrets := slices.Collect(maps.Keys(values))
	// An overlapping shorter credential must not leave the longer one's suffix.
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	pairs := make([]string, 0, 2*len(secrets))
	for _, value := range secrets {
		pairs = append(pairs, value, "[redacted]")
	}
	return strings.NewReplacer(pairs...)
}

var privateKeyBoundary = regexp.MustCompile(`-----(BEGIN|END) (?:[A-Z0-9]+ )*PRIVATE KEY-----`)

// PEM bodies span records. Each input stream tracks its own boundaries.
func (l *StderrLog) emit(line []byte, inPrivateKey *bool) {
	if line == nil || len(line) > maxRuntimeLogLine {
		l.console.offer(l.prefix, "[oversized worker log line omitted]")
		return
	}
	text := strings.TrimSuffix(string(line), "\n")
	if *inPrivateKey || strings.Contains(text, "PRIVATE KEY-----") {
		boundaries := privateKeyBoundary.FindAllStringSubmatch(text, -1)
		hide := *inPrivateKey || len(boundaries) != 0
		for _, boundary := range boundaries {
			*inPrivateKey = boundary[1] == "BEGIN"
		}
		if hide {
			l.console.offer(l.prefix, "[redacted private key]")
			return
		}
	}
	text = ProtectText(l.secrets.Replace(text))
	for len(text) > maxConsoleLine {
		l.console.offer(l.prefix, text[:maxConsoleLine])
		text = text[maxConsoleLine:]
	}
	l.console.offer(l.prefix, text)
}
