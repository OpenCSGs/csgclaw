package connectorcli

import (
	"errors"
	"strings"
	"testing"
)

func TestAuthorizationURLAndErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		value   string
		allowed bool
	}{{"https://open.feishu.cn/device?token=test", true}, {"https://accounts.larksuite.com/device", true}, {"https://feishu.cn.attacker.example/device", false}, {"https://user@open.feishu.cn/device", false}, {"http://open.feishu.cn/device", false}, {"https://open.feishu.cn:8443/device", false}} {
		if got := AllowedAuthorizationURL(tc.value, []string{"feishu.cn", "larksuite.com"}); got != tc.allowed {
			t.Errorf("%q allowed=%v", tc.value, got)
		}
	}
	var result *OperationError
	if err := OperationFailure([]byte(`{"error":"access_denied","access_token":"secret"}`)); !errors.As(err, &result) || result.Code != "authorization_denied" || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error=%v", err)
	}
}

func TestCommandOutputPublishesEachURLOnce(t *testing.T) {
	var urls []string
	w := &commandOutput{domains: []string{"example.com"}, notify: func(value string) { urls = append(urls, value) }}
	for _, chunk := range []string{"Open https://example.com/authorize?code=first\n", "Polling https://example.com/poll\n", "Still waiting\n"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if len(urls) != 2 || urls[0] != "https://example.com/authorize?code=first" || urls[1] != "https://example.com/poll" {
		t.Fatalf("repeated or changed URLs: %v", urls)
	}
}
