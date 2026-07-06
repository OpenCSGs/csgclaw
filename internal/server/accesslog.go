package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/term"
)

var (
	accessLogStderr                 = os.Stderr
	accessLogWriter       io.Writer = os.Stderr
	accessLogIsTerminalFD           = term.IsTerminal
)

const (
	accessLogErrorBodyLimit = 2048
	accessLogRedactedValue  = "[REDACTED]"
)

var accessLogSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+)[^,\s]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|jwt[_-]?token|client[_-]?secret|app[_-]?secret|password|secret)\s*["']?\s*[:=]\s*["']?)[^"',\s}]+`),
}

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

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("uri", accessLogURI(r)),
			slog.Int("status", lw.status),
			slog.Int("bytes", lw.bytes),
			slog.Duration("duration", time.Since(start)),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		}
		if lw.status >= http.StatusBadRequest {
			attrs = append(attrs, accessLogErrorAttrs(r, lw)...)
		}

		logger.LogAttrs(r.Context(), accessLogLevel(lw.status), "http access", attrs...)
	})
}

func accessLogLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func accessLogErrorAttrs(r *http.Request, lw *loggingResponseWriter) []slog.Attr {
	attrs := make([]slog.Attr, 0, 7)
	if msg := accessLogErrorMessage(lw.body, lw.bodyTruncated); msg != "" {
		attrs = append(attrs, slog.String("error", msg))
	}
	if host := strings.TrimSpace(r.Host); host != "" {
		attrs = append(attrs, slog.String("host", host))
	}
	if referer := strings.TrimSpace(r.Referer()); referer != "" {
		attrs = append(attrs, slog.String("referer", accessLogRedactURL(referer)))
	}
	if contentType := strings.TrimSpace(r.Header.Get("Content-Type")); contentType != "" {
		attrs = append(attrs, slog.String("content_type", contentType))
	}
	if r.ContentLength >= 0 {
		attrs = append(attrs, slog.Int64("content_length", r.ContentLength))
	}
	if requestID := accessLogRequestID(r); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	return attrs
}

func accessLogRequestID(r *http.Request) string {
	for _, key := range []string{"X-Request-ID", "X-Correlation-ID"} {
		if value := strings.TrimSpace(r.Header.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func accessLogURI(r *http.Request) string {
	if r == nil {
		return ""
	}
	if r.URL != nil {
		u := *r.URL
		redactURLQuery(&u)
		return u.RequestURI()
	}
	return accessLogRedactURL(r.RequestURI)
}

func accessLogRedactURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return redactRawQuery(raw)
	}
	redactURLQuery(u)
	if u.IsAbs() {
		return u.String()
	}
	return u.RequestURI()
}

func redactURLQuery(u *url.URL) {
	if u == nil || u.RawQuery == "" {
		return
	}
	query := u.Query()
	for key := range query {
		if accessLogSensitiveKey(key) {
			query[key] = []string{accessLogRedactedValue}
		}
	}
	u.RawQuery = query.Encode()
}

func redactRawQuery(raw string) string {
	idx := strings.IndexByte(raw, '?')
	if idx < 0 {
		return raw
	}
	prefix := raw[:idx+1]
	values := strings.Split(raw[idx+1:], "&")
	for i, value := range values {
		key, _, _ := strings.Cut(value, "=")
		if accessLogSensitiveKey(key) {
			values[i] = key + "=" + accessLogRedactedValue
		}
	}
	return prefix + strings.Join(values, "&")
}

func accessLogSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch normalized {
	case "api_key", "access_token", "refresh_token", "id_token", "jwt_token",
		"token", "authorization", "password", "secret", "client_secret",
		"app_secret", "code", "state":
		return true
	default:
		return strings.HasSuffix(normalized, "_token") ||
			strings.HasSuffix(normalized, "_secret") ||
			strings.HasSuffix(normalized, "_password")
	}
}

func accessLogErrorMessage(body []byte, truncated bool) string {
	if len(body) == 0 {
		return ""
	}
	msg := strings.ToValidUTF8(string(body), "?")
	msg = strings.Join(strings.Fields(msg), " ")
	msg = redactLogSecrets(msg)
	if truncated {
		msg += " ... [truncated]"
	}
	return msg
}

func redactLogSecrets(value string) string {
	for _, pattern := range accessLogSecretPatterns {
		value = pattern.ReplaceAllString(value, "${1}"+accessLogRedactedValue)
	}
	return value
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
	statusCode := accessLogStatusCode(values["status"])
	status := colorize(accessLogStatusColor(statusCode), values["status"])

	parts := []string{
		fmt.Sprintf("%s %s %s %s=%s %s=%s",
			method,
			values["uri"],
			status,
			label("bytes"), values["bytes"],
			label("duration"), values["duration"],
		),
	}
	if statusCode >= http.StatusBadRequest {
		appendField := func(key, name string, quote bool) {
			value := strings.TrimSpace(values[key])
			if value == "" {
				return
			}
			if quote {
				parts = append(parts, fmt.Sprintf("%s=%q", label(name), value))
				return
			}
			parts = append(parts, fmt.Sprintf("%s=%s", label(name), value))
		}
		appendField("error", "error", true)
		appendField("remote_addr", "remote", false)
		appendField("host", "host", true)
		appendField("request_id", "request_id", false)
		appendField("content_type", "content_type", true)
		appendField("content_length", "content_length", false)
		appendField("referer", "referer", true)
		appendField("user_agent", "ua", true)
	}

	_, err := fmt.Fprintln(h.writer, strings.Join(parts, " "))
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
	status        int
	bytes         int
	wroteHeader   bool
	body          []byte
	bodyTruncated bool
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	if w.status >= http.StatusBadRequest {
		w.captureBody(p[:n])
	}
	return n, err
}

func (w *loggingResponseWriter) captureBody(p []byte) {
	if len(p) == 0 {
		return
	}
	if len(w.body) >= accessLogErrorBodyLimit {
		w.bodyTruncated = true
		return
	}
	remaining := accessLogErrorBodyLimit - len(w.body)
	if len(p) > remaining {
		w.body = append(w.body, p[:remaining]...)
		w.bodyTruncated = true
		return
	}
	w.body = append(w.body, p...)
}

func (w *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *loggingResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not implement http.Hijacker")
	}
	if !w.wroteHeader {
		w.status = http.StatusSwitchingProtocols
		w.wroteHeader = true
	}
	return hijacker.Hijack()
}
