package template

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

const remoteLogBodyLimit = 4096

const remoteLogOversizedBody = "[body omitted: exceeds 4096-byte log limit] ... [truncated]"

var remoteLogCredentials = regexp.MustCompile(`(?i)((?:["']?(?:[a-z0-9_-]*(?:token|secret|password|api[_-]?key|authorization|cookie)[a-z0-9_-]*)["']?\s*[:=]\s*))(?:"[^"\n]*"|'[^'\n]*'|[^\s,;&}]+)`)
var remoteLogBearer = regexp.MustCompile(`(?i)bearer\s+[^\s"'<>]+`)

func remoteLogSecretKey(key string) bool {
	key = strings.ToLower(key)
	for _, fragment := range []string{"token", "secret", "password", "api_key", "api-key", "apikey", "authorization", "cookie"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func (s *RemoteStore) remoteLogURL(original *url.URL) string {
	u := *original
	u.User = nil
	u.Fragment = ""
	q := u.Query()
	for key := range q {
		if remoteLogSecretKey(key) {
			q.Set(key, "[redacted]")
		}
	}
	u.RawQuery = q.Encode()
	return s.remoteLogText(u.String())
}

func redactRemoteLogJSON(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if remoteLogSecretKey(key) {
				v[key] = "[redacted]"
			} else {
				redactRemoteLogJSON(item)
			}
		}
	case []any:
		for _, item := range v {
			redactRemoteLogJSON(item)
		}
	}
}

func (s *RemoteStore) remoteLogBody(body []byte) string {
	// Check before converting to string, which could copy a large response.
	if len(body) > remoteLogBodyLimit {
		return remoteLogOversizedBody
	}
	return s.remoteLogText(string(body))
}

func (s *RemoteStore) remoteLogText(value string) string {
	// Omit oversized input rather than cutting through a credential or parsing
	// incomplete JSON. All redaction work is bounded by the log input limit.
	if len(value) > remoteLogBodyLimit {
		return remoteLogOversizedBody
	}
	if s.token != "" {
		value = strings.ReplaceAll(value, s.token, "[redacted]")
	}
	var payload any
	if json.Unmarshal([]byte(value), &payload) == nil {
		redactRemoteLogJSON(payload)
		if data, err := json.Marshal(payload); err == nil {
			value = string(data)
		}
	}
	value = remoteLogCredentials.ReplaceAllString(value, "${1}[redacted]")
	value = remoteLogBearer.ReplaceAllString(value, "Bearer [redacted]")
	if len(value) > remoteLogBodyLimit {
		value = value[:remoteLogBodyLimit] + " ... [truncated]"
	}
	return strings.ToValidUTF8(value, "?")
}
