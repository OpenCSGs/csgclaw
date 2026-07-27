package serve

import (
	"net"
	"reflect"
	"testing"
)

func TestDesktopServerAccessHostsUsesSamePortHTTPRuntime(t *testing.T) {
	listenerAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 59842}

	got := desktopServerAccessHosts("http://host.docker.internal:59842", listenerAddr)
	want := []string{"host.docker.internal:59842"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("desktopServerAccessHosts() = %q, want %q", got, want)
	}
}

func TestDesktopServerAccessHostsRejectsUnrelatedRuntimeURLs(t *testing.T) {
	listenerAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 59842}
	for _, runtimeBaseURL := range []string{
		"https://host.docker.internal:59842",
		"http://host.docker.internal:18080",
		"http://user@host.docker.internal:59842",
		"not a URL",
	} {
		t.Run(runtimeBaseURL, func(t *testing.T) {
			if got := desktopServerAccessHosts(runtimeBaseURL, listenerAddr); len(got) != 0 {
				t.Fatalf("desktopServerAccessHosts(%q) = %q, want empty", runtimeBaseURL, got)
			}
		})
	}
}
