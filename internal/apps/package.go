package apps

import (
	"csgclaw/internal/connectorcli"
	skilllocal "csgclaw/internal/skill/local"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

type mcpPackage struct {
	Servers     map[string]Config `json:"mcpServers"`
	AuthMethods []string          `json:"auth_methods,omitempty"`
}

func (p mcpPackage) config() Config {
	for _, config := range p.Servers {
		return config
	}
	return Config{}
}

type pluginPackage struct {
	Manifest   Manifest                `json:"manifest"`
	Apps       map[string]AppReference `json:"apps,omitempty"`
	MCP        *mcpPackage             `json:"mcp,omitempty"`
	CLI        *connectorcli.Spec      `json:"cli,omitempty"`
	SkillFiles map[string][]byte       `json:"skill_files,omitempty"`
}

func loadPackages() (map[string]pluginPackage, error) {
	source, err := fs.Sub(builtins, "builtin")
	if err != nil {
		return nil, err
	}
	return readPackages(source)
}
func readPackages(source fs.FS) (map[string]pluginPackage, error) {
	result := map[string]pluginPackage{}
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root, err := fs.Sub(source, entry.Name())
		if err != nil {
			return nil, err
		}
		pkg, err := readPackage(root)
		if err != nil {
			return nil, fmt.Errorf("connector %s: %w", entry.Name(), err)
		}
		id := pkg.Manifest.Name
		if _, ok := result[id]; ok {
			return nil, fmt.Errorf("duplicate connector %s", id)
		}
		result[id] = pkg
	}
	return result, nil
}
func readPackage(source fs.FS) (pluginPackage, error) {
	var pkg pluginPackage
	read := func(ref string, value any) error {
		ref = strings.TrimPrefix(ref, "./")
		if !fs.ValidPath(ref) || ref == "." || strings.Contains(ref, "\\") {
			return fmt.Errorf("invalid package reference")
		}
		data, err := fs.ReadFile(source, ref)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, value)
	}
	if err := read("plugin.json", &pkg.Manifest); err != nil {
		return pkg, err
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`).MatchString(pkg.Manifest.Name) {
		return pkg, fmt.Errorf("invalid connector name")
	}
	if (pkg.Manifest.Kind == "" || pkg.Manifest.Kind == KindMCP) && pkg.Manifest.Apps != "" && pkg.Manifest.MCP == "" && pkg.Manifest.CLI == "" {
		var legacy struct {
			Apps map[string]AppReference `json:"apps"`
		}
		if err := read(pkg.Manifest.Apps, &legacy); err != nil {
			return pkg, err
		}
		pkg.Apps = legacy.Apps
		if len(pkg.Apps) != 1 {
			return pkg, fmt.Errorf("legacy package must reference one service")
		}
		for _, app := range pkg.Apps {
			if !app.Required || !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`).MatchString(app.ID) {
				return pkg, fmt.Errorf("invalid legacy service reference")
			}
			pkg.Manifest.Name = app.ID
		}
		pkg.Manifest.Kind = KindMCP
		pkg.MCP = &mcpPackage{AuthMethods: []string{"none", "bearer", "header", "env"}}
	} else {
		if strings.TrimSpace(pkg.Manifest.Version) == "" {
			return pkg, fmt.Errorf("connector package version is required")
		}
		switch pkg.Manifest.Kind {
		case KindMCP:
			if pkg.Manifest.CLI != "" || pkg.Manifest.MCP == "" {
				return pkg, fmt.Errorf("MCP package requires only an MCP configuration")
			}
			if err := read(pkg.Manifest.MCP, &pkg.MCP); err != nil {
				return pkg, err
			}
			if pkg.MCP == nil || len(pkg.MCP.Servers) != 1 {
				return pkg, fmt.Errorf("MCP package must define one server")
			}
			config := normalizeConfig(pkg.MCP.config(), pkg.Manifest.Name)
			if err := validateConfig(config, pkg.Manifest.Name); err != nil {
				return pkg, err
			}
			if len(pkg.MCP.AuthMethods) == 0 {
				pkg.MCP.AuthMethods = []string{"none", "bearer", "header", "env"}
			}
		case KindCLI:
			if pkg.Manifest.MCP != "" || pkg.Manifest.CLI == "" {
				return pkg, fmt.Errorf("CLI package requires only a CLI configuration")
			}
			if err := read(pkg.Manifest.CLI, &pkg.CLI); err != nil {
				return pkg, err
			}
			if pkg.CLI == nil {
				return pkg, fmt.Errorf("CLI configuration is required")
			}

			if err := pkg.CLI.Validate(); err != nil {
				return pkg, err
			}
		default:
			return pkg, fmt.Errorf("unsupported connector kind")
		}
	}
	if len(pkg.Manifest.Skills) > 0 {
		if pkg.CLI != nil && pkg.CLI.Install.Skills.URL != "" {
			return pkg, fmt.Errorf("choose packaged Skills or a Skills archive")
		}
		ref := pkg.Manifest.Skills
		ref = strings.TrimPrefix(ref, "./")
		if !fs.ValidPath(ref) || ref == "." || strings.Contains(ref, "\\") {
			return pkg, fmt.Errorf("invalid Skills reference")
		}
		pkg.SkillFiles = map[string][]byte{}
		var size int
		err := fs.WalkDir(source, ref, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("Skills must contain regular files")
			}
			data, err := fs.ReadFile(source, name)
			if err != nil {
				return err
			}
			size += len(data)
			if size > 64<<20 {
				return fmt.Errorf("Skills exceed size limit")
			}
			relative := strings.TrimPrefix(name, ref+"/")
			if relative == path.Base(relative) {
				return fmt.Errorf("Skills files require a named directory")
			}
			pkg.SkillFiles[relative] = data
			return nil
		})
		if err != nil {
			return pkg, err
		}
	}
	for name := range pkg.SkillFiles {
		skillName, _, _ := strings.Cut(name, "/")
		normalized, err := skilllocal.NormalizeName(skillName)
		if err != nil || normalized != skillName {
			return pkg, fmt.Errorf("invalid Skill name")
		}
		if _, ok := pkg.SkillFiles[skillName+"/SKILL.md"]; !ok {
			return pkg, fmt.Errorf("Skill %s requires SKILL.md", skillName)
		}
	}
	if pkg.Manifest.Interface.DisplayName == "" {
		pkg.Manifest.Interface.DisplayName = pkg.Manifest.Name
	}
	return pkg, nil
}

func (s *Service) packageConfig(appID string, config Config, credentials Credentials) Config {
	pkg := s.packages[appID]
	if pkg.MCP != nil {
		// User-supplied fields replace package defaults, when provided.
		defaults := pkg.MCP.config()
		if config.Command != "" || config.Transport == "stdio" {
			defaults.URL = ""
			defaults.Transport = "stdio"
		}
		if config.URL != "" || config.Transport == "http" {
			defaults.Command = ""
			defaults.Args = nil
			defaults.Transport = "http"
		}
		base, _ := json.Marshal(defaults)
		supplied, _ := json.Marshal(config)
		values := map[string]json.RawMessage{}
		_ = json.Unmarshal(base, &values)
		overrides := map[string]json.RawMessage{}
		_ = json.Unmarshal(supplied, &overrides)
		for key, value := range overrides {
			values[key] = value
		}
		merged, _ := json.Marshal(values)
		_ = json.Unmarshal(merged, &config)
	}
	return normalizeConfig(config, appID, credentials)
}
