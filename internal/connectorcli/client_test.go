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
