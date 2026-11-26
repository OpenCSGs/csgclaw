package runtime

import (
	"net/url"
	stdruntime "runtime"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/http/httpproxy"
)

func TestWithLocalProxyBypass(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environment []string
		callbackURL string
		want        []string
		wantWindows []string
	}{
		{
			name:        "no existing exclusions",
			environment: []string{"HTTP_PROXY=http://proxy.example:8080"},
			callbackURL: "http://127.0.0.1:18080",
			want:        []string{"HTTP_PROXY=http://proxy.example:8080", "NO_PROXY=127.0.0.1,localhost,::1,[::1]"},
		},
		{
			name:        "existing rules and custom listener",
			environment: []string{"NO_PROXY=.vpn.example,127.0.0.1", "no_proxy=192.168.0.0/16, LOCALHOST"},
			callbackURL: "http://192.168.1.2:18080",
			want:        []string{"NO_PROXY=.vpn.example,127.0.0.1,localhost,::1,[::1],192.168.1.2", "no_proxy=192.168.0.0/16, LOCALHOST,127.0.0.1,::1,[::1],192.168.1.2"},
		},
		{
			name:        "wildcard",
			environment: []string{"no_proxy=*"},
			want:        []string{"no_proxy=*"},
		},
		{
			name:        "empty lowercase falls back to uppercase",
			environment: []string{"NO_PROXY=fixture.invalid", "no_proxy="},
			want:        []string{"NO_PROXY=fixture.invalid,127.0.0.1,localhost,::1,[::1]", "no_proxy=fixture.invalid,127.0.0.1,localhost,::1,[::1]"},
		},
		{
			name:        "empty uppercase falls back to lowercase",
			environment: []string{"NO_PROXY=", "no_proxy=fixture.invalid"},
			want:        []string{"NO_PROXY=fixture.invalid,127.0.0.1,localhost,::1,[::1]", "no_proxy=fixture.invalid,127.0.0.1,localhost,::1,[::1]"},
		},
		{
			name:        "empty lowercase falls back to wildcard",
			environment: []string{"NO_PROXY=*", "no_proxy="},
			want:        []string{"NO_PROXY=*", "no_proxy=*"},
		},
		{
			name:        "whitespace lowercase keeps its own exclusions",
			environment: []string{"NO_PROXY=*", "no_proxy= "},
			want:        []string{"NO_PROXY=*,127.0.0.1,localhost,::1,[::1]", "no_proxy= ,127.0.0.1,localhost,::1,[::1]"},
			wantWindows: []string{"NO_PROXY=*", "no_proxy= ,127.0.0.1,localhost,::1,[::1]"},
		},
		{
			name:        "IPv6 listener",
			environment: []string{"no_proxy="},
			callbackURL: "http://[fd00::1]:18080",
			want:        []string{"no_proxy=127.0.0.1,localhost,::1,[::1],fd00::1,[fd00::1]"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			if stdruntime.GOOS == "windows" && tc.wantWindows != nil {
				want = tc.wantWindows
			}
			got := WithLocalProxyBypass(tc.environment, tc.callbackURL)
			if !slices.Equal(got, want) {
				t.Fatalf("environment = %q, want %q", got, want)
			}
			if again := WithLocalProxyBypass(got, tc.callbackURL); !slices.Equal(again, want) {
				t.Fatalf("environment after adding exclusions again = %q, want %q", again, want)
			}
		})
	}
}

func TestRuntimeProxyBypassKeepsWhitespaceRouting(t *testing.T) {
	const callbackURL = "http://192.0.2.10:18080"
	for _, tc := range []struct {
		key   string
		alias string
	}{
		{key: "no_proxy", alias: "NO_PROXY"},
		{key: "NO_PROXY", alias: "no_proxy"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			base := []string{tc.alias + "=*", tc.key + "= "}
			if stdruntime.GOOS != "windows" {
				// The order of the inherited entries must not select a spelling.
				slices.Reverse(base)
			}
			environment, _, err := MergeExtensionEnvironment(base, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			environment = WithLocalProxyBypass(environment, callbackURL)
			prefix := tc.key + "="
			index := slices.IndexFunc(environment, func(entry string) bool { return strings.HasPrefix(entry, prefix) })
			if index < 0 {
				t.Fatalf("missing %s in %q", tc.key, environment)
			}
			config := httpproxy.Config{HTTPProxy: "http://proxy.example:8080", NoProxy: strings.TrimPrefix(environment[index], prefix)}
			proxyFor := config.ProxyFunc()
			for _, target := range []struct {
				url        string
				wantsProxy bool
			}{
				{url: "http://model.example/v1", wantsProxy: true},
				{url: callbackURL},
			} {
				request, err := url.Parse(target.url)
				if err != nil {
					t.Fatal(err)
				}
				proxy, err := proxyFor(request)
				if err != nil || (proxy != nil) != target.wantsProxy {
					t.Errorf("proxy for %s = %v, error = %v, want proxy: %t", target.url, proxy, err, target.wantsProxy)
				}
			}
		})
	}
}
