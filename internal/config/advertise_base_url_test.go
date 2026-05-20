package config

import "testing"

func TestAdvertiseBaseURLForClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		server ServerConfig
		want   string
	}{
		{
			name:   "configured",
			server: ServerConfig{AdvertiseBaseURL: "https://csgclaw.example.com/"},
			want:   "https://csgclaw.example.com",
		},
		{
			name:   "empty uses loopback and listen port",
			server: ServerConfig{ListenAddr: "127.0.0.1:19090"},
			want:   "http://127.0.0.1:19090",
		},
		{
			name:   "empty uses default http port",
			server: ServerConfig{},
			want:   "http://127.0.0.1:" + DefaultHTTPPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AdvertiseBaseURLForClient(tt.server); got != tt.want {
				t.Fatalf("AdvertiseBaseURLForClient() = %q, want %q", got, tt.want)
			}
		})
	}
}
