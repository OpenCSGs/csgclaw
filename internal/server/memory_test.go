package server

import "testing"

func TestAgentMemoryDefaultsOnWithoutProvisioningMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, bind, sandbox, purpose, tenant, user, token string
		desktop                                           bool
	}{
		{name: "loopback", bind: "127.0.0.1:18080"},
		{name: "all IPv4 interfaces", bind: "0.0.0.0:18080"},
		{name: "all IPv6 interfaces", bind: "[::]:18080"},
		{name: "LAN", bind: "192.0.2.1:18080"},
		{name: "desktop", desktop: true},
		{name: "cloud without personal flag", sandbox: "csgclaw"},
		{name: "cloud workspace", sandbox: "csgclaw-workspace"},
		{name: "shared deployment", sandbox: "csgclaw", purpose: "shared"},
		{name: "partial owner metadata", sandbox: "csgclaw", tenant: "owner-a"},
		{name: "mismatched owner metadata", sandbox: "csgclaw", tenant: "owner-a", user: "owner-b", token: "fixture"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range map[string]string{
				"OPENCSG_SANDBOX_TYPE": tc.sandbox, "CSGCLAW_INSTANCE_KIND": tc.purpose,
				"CSGCLAW_TENANT_ID": tc.tenant, "CSGHUB_USER_UUID": tc.user,
				"CSGHUB_USER_TOKEN": tc.token, "CSGCLAW_NAME": "",
			} {
				t.Setenv(key, value)
			}
			opts := Options{NoAuth: true, ListenAddr: tc.bind}
			want := "local"
			if tc.desktop {
				opts.Desktop = &DesktopOptions{}
				want = "desktop"
			}
			if got := agentMemoryMode(opts); got != want {
				t.Fatalf("mode=%q want %q", got, want)
			}
		})
	}
}
