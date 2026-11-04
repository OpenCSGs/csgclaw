package apps

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"csgclaw/internal/connectorcli"
)

func (s *Service) installCLI(ctx context.Context, root string, spec connectorcli.InstallSpec, progress func(string)) (connectorcli.Package, error) {
	if s.options.CLI != nil && s.options.CLI.Install != nil {
		return s.options.CLI.Install(ctx, root, progress)
	}
	return (connectorcli.Installer{Root: root, Spec: s.currentCLIInstallSpec(spec), PreinstalledDir: os.Getenv("CSGCLAW_CONNECTOR_PACKAGES_DIR")}).Install(ctx, progress)
}

// Saved connectors pin package bytes, but must still receive corrected download
// locations for those same bytes when a failed installation is retried.
func (s *Service) currentCLIInstallSpec(spec connectorcli.InstallSpec) connectorcli.InstallSpec {
	for _, pkg := range s.packages {
		if pkg.CLI == nil || pkg.CLI.Install.Version != spec.Version || pkg.CLI.Install.Executable != spec.Executable {
			continue
		}
		spec.Releases = maps.Clone(spec.Releases)
		for platform, archive := range spec.Releases {
			if current, ok := pkg.CLI.Install.Releases[platform]; ok && current.SHA256 == archive.SHA256 {
				archive.URL = current.URL
				spec.Releases[platform] = archive
			}
		}
		break
	}
	return spec
}

// PrepareCLIPackage 使用当前内置声明生成镜像预装包。
func PrepareCLIPackage(ctx context.Context, appID, directory string) (connectorcli.Package, error) {
	packages, err := loadPackages()
	if err != nil {
		return connectorcli.Package{}, err
	}
	pkg, ok := packages[appID]
	if !ok || pkg.CLI == nil {
		return connectorcli.Package{}, fmt.Errorf("%w: CLI connector %s", ErrNotFound, appID)
	}
	if directory == "" {
		return connectorcli.Package{}, fmt.Errorf("package directory is required")
	}
	root, err := filepath.Abs(filepath.Join(directory, pkg.CLI.Install.Executable))
	if err != nil {
		return connectorcli.Package{}, err
	}
	return (connectorcli.Installer{Root: root, Spec: pkg.CLI.Install}).Install(ctx, nil)
}
