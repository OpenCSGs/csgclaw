package apps

import (
	"runtime"
	"slices"

	"csgclaw/internal/connectorcli"
)

// Refresh URL recognition for saved commands using the same pinned executable.
func (s *Service) currentCLIAuthorizationSpec(appID string, saved *connectorcli.Spec) *connectorcli.Spec {
	pkg, ok := s.packages[appID]
	if !ok || pkg.CLI == nil || saved == nil || pkg.CLI.Install.Version != saved.Install.Version || pkg.CLI.Install.Executable != saved.Install.Executable {
		return saved
	}
	platform := runtime.GOOS + "-" + runtime.GOARCH
	currentArchive, currentOK := pkg.CLI.Install.Releases[platform]
	savedArchive, savedOK := saved.Install.Releases[platform]
	if !currentOK || !savedOK || currentArchive.SHA256 != savedArchive.SHA256 {
		return saved
	}
	updated := copyJSON(saved)
	for _, pair := range [][2]connectorcli.Commands{
		{updated.Init, pkg.CLI.Init}, {updated.Auth, pkg.CLI.Auth},
		{updated.UnAuth, pkg.CLI.UnAuth}, {updated.Disconnect, pkg.CLI.Disconnect},
	} {
		for i := range pair[0] {
			for _, current := range pair[1] {
				if current.AuthorizationURLPattern != "" && slices.Equal(pair[0][i].Args, current.Args) {
					pair[0][i].AuthorizationURLPattern = current.AuthorizationURLPattern
					break
				}
			}
		}
	}
	return updated
}
