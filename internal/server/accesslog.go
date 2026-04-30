package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

var (
	accessLogStderr                 = os.Stderr
	accessLogWriter       io.Writer = os.Stderr
	accessLogIsTerminalFD           = term.IsTerminal
)

func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
	logger = newAccessLogger(logger)
	if next == nil {
		next = http.NotFoundHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accessLogShouldSkip(r) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		lw := &loggingResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(lw, r)

		logger.Info("http access",
			"method", r.Method,
			"uri", r.RequestURI,
			"status", lw.status,
			"bytes", lw.bytes,
			"duration", time.Since(start),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)
	})
}

func accessLogShouldSkip(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	if r.URL.Path == "/healthz" {
		return true
	}
	if r.Method != http.MethodGet {
		return false
	}
	return r.URL.Path == "/api/v1/agents" && r.URL.Query().Get("poll") == "1"
}

const (
	accessLogColorReset  = "\033[0m"
	accessLogLabelColor  = "\033[90m"
	accessLogMethodColor = "\033[36m"
	accessLogStatus2xx   = "\033[32m"
	accessLogStatus3xx   = "\033[34m"
	accessLogStatus4xx   = "\033[33m"
	accessLogStatus5xx   = "\033[31m"
	accessLogStatusOther = "\033[35m"
)

func newAccessLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	if !accessLogUsesTerminalColor() {
		return logger
	}
	return slog.New(&accessLogTerminalHandler{
		writer: accessLogWriter,
		level:  slog.LevelInfo,
	})
}

func accessLogUsesTerminalColor() bool {
	if accessLogStderr == nil {
		return false
	}
	if accessLogWriter == nil {
		return false
	}
	return accessLogIsTerminalFD(int(accessLogStderr.Fd()))
}

func accessLogStatusColor(status int) string {
	switch {
	case status >= 200 && status < 300:
		return accessLogStatus2xx
	case status >= 300 && status < 400:
		return accessLogStatus3xx
	case status >= 400 && status < 500:
		return accessLogStatus4xx
	case status >= 500 && status < 600:
		return accessLogStatus5xx
	default:
		return accessLogStatusOther
	}
}

func colorize(color, value string) string {
	return color + value + accessLogColorReset
}

type accessLogTerminalHandler struct {
	writer io.Writer
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
}

func (h *accessLogTerminalHandler) Enabled(_ context.Context, level slog.Level) bool {
	threshold := slog.LevelInfo
	if h != nil && h.level != nil {
		threshold = h.level.Level()
	}
	return level >= threshold
}

func (h *accessLogTerminalHandler) Handle(_ context.Context, record slog.Record) error {
	if h == nil || h.writer == nil {
		return nil
	}

	attrs := make([]slog.Attr, 0, len(h.attrs)+record.NumAttrs())
	attrs = append(attrs, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})

	values := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		key := qualifyAccessLogKey(h.groups, attr.Key)
		values[key] = accessLogAttrValue(attr.Value)
	}

	label := func(v string) string { return colorize(accessLogLabelColor, v) }
	method := colorize(accessLogMethodColor, values["method"])
	status := colorize(accessLogStatusColor(accessLogStatusCode(values["status"])), values["status"])

	_, err := fmt.Fprintf(h.writer, "%s %s %s %s=%s %s=%s %s=%s %s=%q\n",
		method,
		values["uri"],
		status,
		label("bytes"), values["bytes"],
		label("duration"), values["duration"],
		label("remote"), values["remote_addr"],
		label("ua"), values["user_agent"],
	)
	return err
}

func (h *accessLogTerminalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *accessLogTerminalHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func qualifyAccessLogKey(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	return strings.Join(append(append([]string{}, groups...), key), ".")
}

func accessLogAttrValue(value slog.Value) string {
	return fmt.Sprint(value.Any())
}

func accessLogStatusCode(value string) int {
	var status int
	_, _ = fmt.Sscanf(value, "%d", &status)
	return status
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status = status
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (w *loggingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
