package apps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"csgclaw/internal/connectorcli"
)

func (s *Service) installCLI(ctx context.Context, root string, spec connectorcli.InstallSpec, progress func(string)) (connectorcli.Package, error) {
	if s.options.CLI != nil && s.options.CLI.Install != nil {
		return s.options.CLI.Install(ctx, root, progress)
	}
	return (connectorcli.Installer{Root: root, Spec: spec, PreinstalledDir: os.Getenv("CSGCLAW_CONNECTOR_PACKAGES_DIR")}).Install(ctx, progress)
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
