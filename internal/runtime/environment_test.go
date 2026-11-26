package runtime

import (
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
