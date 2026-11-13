package updates

import (
	"csgclaw/internal/skill/installstate"
	"testing"
)

func TestNewerReleaseOrdering(t *testing.T) {
	for _, test := range []struct {
		latest, current string
		want            bool
	}{
		{"1.10.0", "1.9.0", true}, {"v1.1.0", "1.0.0", true}, {"1.0.0", "1.0.0-beta.2", true}, {"1.0.0-beta.10", "1.0.0-beta.2", true},
		{"1.0.0+build.2", "1.0.0+build.1", false}, {"1.0.0", "2.0.0", false}, {"1.0.0", "", false}, {".1.0.0", "1.0.0", false},
	} {
		if got := Newer(test.latest, test.current); got != test.want {
			t.Errorf("Newer(%q,%q)=%v", test.latest, test.current, got)
		}
	}
}

func TestSourcePathRequiresExactHubAndRepository(t *testing.T) {
	for _, test := range []struct {
		source, path string
		want         bool
	}{
		{"https://hub.example/skills/team/writer", "team/writer", true},
		{"https://hub.example/skills/team/writer", "other/writer", false},
		{"https://another.example/skills/team/writer", "team/writer", false},
		{"https://hub.example.evil/skills/team/writer", "team/writer", false},
	} {
		_, err := SourcePath(installstate.Record{Source: installstate.SourceMarket, SourceURL: test.source, RemotePath: test.path}, "https://hub.example")
		if (err == nil) != test.want {
			t.Errorf("source=%q path=%q err=%v", test.source, test.path, err)
		}
	}
}
