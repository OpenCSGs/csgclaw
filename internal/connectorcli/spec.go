package connectorcli

import (
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
)

type Archive struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}
type SkillArchive struct {
	Archive
	Version string `json:"version"`
	Prefix  string `json:"prefix,omitempty"`
}
type InstallSpec struct {
	Version     string             `json:"version"`
	Executable  string             `json:"executable"`
	VersionArgs []string           `json:"version_args"`
	Releases    map[string]Archive `json:"releases"`
	Skills      SkillArchive       `json:"skills,omitempty"`
}
type Spec struct {
	Install               InstallSpec       `json:"install"`
	Environment           map[string]string `json:"environment"`
	AuthorizationDomains  []string          `json:"authorization_domains,omitempty"`
	Init                  Commands          `json:"init,omitempty"`
	Auth                  Commands          `json:"auth,omitempty"`
	Status                []string          `json:"status"`
	StatusMatch           string            `json:"status_match,omitempty"`
	StatusMatchJSON       JSONMatch         `json:"status_match_json,omitempty"`
	StatusUnavailableJSON JSONMatch         `json:"status_unavailable_json,omitempty"`
	ExcludeEnvironment    []string          `json:"exclude_environment,omitempty"`
	StatusFields          []string          `json:"status_fields,omitempty"`
	RequestedScopes       string            `json:"requested_scopes,omitempty"`
	UnAuth                Commands          `json:"un_auth,omitempty"`
	Disconnect            Commands          `json:"disconnect,omitempty"`
}

func (s Spec) Validate() error {
	if err := s.Install.Validate(); err != nil {
		return err
	}
	if len(s.Auth) > 0 && len(s.UnAuth) == 0 {
		return errors.New("authorized CLI requires a logout command")
	}
	if len(s.Status) == 0 || (s.StatusMatch == "" && len(s.StatusMatchJSON) == 0) {
		return errors.New("CLI requires a status command and match expression")
	}
	if _, err := regexp.Compile(s.StatusMatch); err != nil {
		return err
	}

	if err := s.StatusUnavailableJSON.Validate(); err != nil {
		return err
	}
	if err := s.StatusMatchJSON.Validate(); err != nil {
		return err
	}
	for _, pointer := range s.StatusFields {
		if _, err := pointerParts(pointer); err != nil {
			return err
		}
	}
	for _, commands := range []Commands{s.Init, s.Auth, s.UnAuth, s.Disconnect} {
		if err := commands.Validate(); err != nil {
			return err
		}
	}
	for _, pattern := range s.ExcludeEnvironment {
		if _, err := path.Match(pattern, ""); err != nil {
			return err
		}
	}
	for key, value := range s.Environment {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(key) || strings.ContainsRune(value, 0) {
			return errors.New("invalid CLI environment")
		}
	}
	for _, domain := range s.AuthorizationDomains {
		u, err := url.Parse("https://" + domain)
		if err != nil || u.Hostname() != domain || u.Port() != "" || u.Path != "" || u.User != nil || domain == "" {
			return errors.New("invalid authorization domain")
		}
	}
	return nil
}
func (s InstallSpec) Validate() error {
	if s.Version == "" || path.Base(s.Version) != s.Version || strings.ContainsAny(s.Version, "\\") || s.Version == "." || s.Version == ".." {
		return errors.New("invalid CLI version")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`).MatchString(s.Executable) {
		return errors.New("invalid CLI executable")
	}
	if len(s.VersionArgs) == 0 || len(s.Releases) == 0 {
		return errors.New("CLI installation requires releases and version arguments")
	}
	for _, archive := range s.Releases {
		if err := archive.validate(); err != nil {
			return err
		}
	}
	if s.Skills.URL != "" {
		if s.Skills.Version == "" {
			return errors.New("Skills archive requires a version")
		}
		if err := s.Skills.Archive.validate(); err != nil {
			return err
		}
	}
	return nil
}
func (a Archive) validate() error {
	u, err := url.Parse(a.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("package URL must use HTTPS")
	}
	sum, err := hex.DecodeString(a.SHA256)
	if err != nil || len(sum) != 32 {
		return errors.New("package requires a SHA256 checksum")
	}
	return nil
}
func (s Spec) Env(configDir string) map[string]string {
	env := make(map[string]string, len(s.Environment))
	for key, value := range s.Environment {
		env[key] = strings.ReplaceAll(value, "${CONFIG_DIR}", configDir)
	}
	return env
}

// ExcludesEnvironment applies the package's patterns to inherited variables.
func (s Spec) ExcludesEnvironment(key string) bool {
	for _, pattern := range s.ExcludeEnvironment {
		if matched, _ := path.Match(pattern, key); matched {
			return true
		}
	}
	return false
}
func (s Spec) StatusUnavailable(data []byte) bool {
	return matchesOutput(data, s.StatusUnavailableJSON)
}
