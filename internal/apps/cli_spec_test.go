package apps

import (
	"reflect"
	"runtime"
	"testing"
)

func TestSavedCLIURLPoliciesPreservePinnedConfiguration(t *testing.T) {
	s, _, _ := newCLIFixture(t)
	for _, variant := range []string{"same executable", "different version", "different checksum", "different command"} {
		t.Run(variant, func(t *testing.T) {
			saved := copyJSON(s.packages["feishu"].CLI)
			saved.Init[0].AuthorizationURLPattern = ""
			saved.RequestedScopes = "saved:scope"
			saved.Init[0].URLRewrite.Query["source"] = "saved-source"
			switch variant {
			case "different version":
				saved.Install.Version = "other-version"
			case "different checksum":
				platform := runtime.GOOS + "-" + runtime.GOARCH
				archive := saved.Install.Releases[platform]
				archive.SHA256 = "other-checksum"
				saved.Install.Releases[platform] = archive
			case "different command":
				saved.Init[0].Args = []string{"custom-init"}
			}
			original := copyJSON(saved)
			got := s.currentCLIAuthorizationSpec("feishu", saved)
			if variant == "same executable" {
				if got.Init[0].AuthorizationURLPattern == "" {
					t.Fatal("saved connector did not receive the authorization URL policy")
				}
			} else if got.Init[0].AuthorizationURLPattern != "" {
				t.Fatal("unrelated command received a new URL policy")
			}
			if !reflect.DeepEqual(saved, original) || got.RequestedScopes != original.RequestedScopes || !reflect.DeepEqual(got.Init[0].URLRewrite, original.Init[0].URLRewrite) || !reflect.DeepEqual(got.Install, original.Install) {
				t.Fatal("URL policy refresh changed pinned configuration")
			}
		})
	}
}
