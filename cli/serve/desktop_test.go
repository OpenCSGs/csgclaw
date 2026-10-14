package serve

import (
	"net"
	"reflect"
	"testing"
)

func TestListenDesktopEndpointsSeparatesRendererAndSandboxBindings(t *testing.T) {
	var requested []string
	rendererListener, sandboxListener, err := listenDesktopEndpointsWith(func(network, address string) (net.Listener, error) {
		requested = append(requested, address)
		// Check the production address without claiming a running Desktop's port.
		if address == desktopRendererListenAddr {
			address = "127.0.0.1:0"
		}
		return net.Listen(network, address)
	})
	if err != nil {
		t.Fatalf("listenDesktopEndpoints() error = %v", err)
	}
	defer rendererListener.Close()
	defer sandboxListener.Close()

	rendererAddr, ok := rendererListener.Addr().(*net.TCPAddr)
	if !ok || !rendererAddr.IP.IsLoopback() {
		t.Fatalf("renderer address = %v, want loopback TCP address", rendererListener.Addr())
	}
	if !reflect.DeepEqual(requested, []string{desktopRendererListenAddr, "0.0.0.0:0"}) {
		t.Fatalf("requested addresses = %q, want stable renderer and ephemeral sandbox", requested)
	}
	sandboxAddr, ok := sandboxListener.Addr().(*net.TCPAddr)
	if !ok || !sandboxAddr.IP.IsUnspecified() {
		t.Fatalf("sandbox address = %v, want wildcard TCP address", sandboxListener.Addr())
	}
	if rendererAddr.Port == sandboxAddr.Port {
		t.Fatalf("renderer and sandbox unexpectedly share port %d", rendererAddr.Port)
	}
}

func TestDesktopServerAccessHostsUsesSandboxPort(t *testing.T) {
	listenerAddr := &net.TCPAddr{IP: net.IPv4zero, Port: 59842}

	got := desktopServerAccessHosts("http://host.docker.internal:59842", listenerAddr)
	want := []string{"127.0.0.1:59842", "host.docker.internal:59842"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("desktopServerAccessHosts() = %q, want %q", got, want)
	}
}

func TestDesktopServerAccessHostsAllowsLinuxAndNonDockerLANAddress(t *testing.T) {
	listenerAddr := &net.TCPAddr{IP: net.IPv4zero, Port: 59842}

	got := desktopServerAccessHosts("http://192.168.1.24:59842", listenerAddr)
	want := []string{"127.0.0.1:59842", "192.168.1.24:59842"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("desktopServerAccessHosts() = %q, want %q", got, want)
	}
}

func TestDesktopServerAccessHostsRejectsUnrelatedRuntimeURLs(t *testing.T) {
	listenerAddr := &net.TCPAddr{IP: net.IPv4zero, Port: 59842}
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
