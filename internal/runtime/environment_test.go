package runtime

import (
	"net/http"
	"os"
	"os/exec"
	stdruntime "runtime"
	"slices"
	"testing"
)

func TestWithLocalProxyBypass(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environment []string
		callbackURL string
		want        []string
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
			want:        []string{"NO_PROXY=*", "no_proxy= ,127.0.0.1,localhost,::1,[::1]"},
		},
		{
			name:        "IPv6 listener",
			environment: []string{"no_proxy="},
			callbackURL: "http://[fd00::1]:18080",
			want:        []string{"no_proxy=127.0.0.1,localhost,::1,[::1],fd00::1,[fd00::1]"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := WithLocalProxyBypass(tc.environment, tc.callbackURL)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("environment = %q, want %q", got, tc.want)
			}
			if again := WithLocalProxyBypass(got, tc.callbackURL); !slices.Equal(again, tc.want) {
				t.Fatalf("environment after adding exclusions again = %q, want %q", again, tc.want)
			}
		})
	}
}

func TestRuntimeProxyBypassKeepsWhitespaceRouting(t *testing.T) {
	const callbackURL = "http://192.0.2.10:18080"
	const proxyURL = "http://proxy.example:8080"
	const childMarker = "CSGCLAW_TEST_PROXY_BYPASS_CHILD"
	if os.Getenv(childMarker) == "1" {
		for _, tc := range []struct {
			target string
			want   string
		}{
			{target: "http://model.example/v1", want: proxyURL},
			{target: callbackURL},
		} {
			request, err := http.NewRequest(http.MethodGet, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			proxy, err := http.ProxyFromEnvironment(request)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if proxy != nil {
				got = proxy.String()
			}
			if got != tc.want {
				t.Errorf("proxy for %s = %q, want %q", tc.target, got, tc.want)
			}
		}
		return
	}

	type environmentCase struct {
		name string
		base []string
	}
	cases := []environmentCase{
		{name: "lowercase last", base: []string{"NO_PROXY=*", "no_proxy= "}},
	}
	if stdruntime.GOOS != "windows" {
		cases = append(cases, environmentCase{name: "lowercase first", base: []string{"no_proxy= ", "NO_PROXY=*"}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := append(tc.base, "HTTP_PROXY="+proxyURL)
			environment, _, err := MergeExtensionEnvironment(base, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			environment = WithLocalProxyBypass(environment, callbackURL)
			// net/http caches its environment on first use.
			child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRuntimeProxyBypassKeepsWhitespaceRouting$")
			child.Env = append(environment, childMarker+"=1")
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("proxy routing in worker environment %q: %v\n%s", environment, err, output)
			}
		})
	}
}
