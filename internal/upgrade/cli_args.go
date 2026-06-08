package upgrade

import "strings"

// commandArgsWithConfig prefixes CLI args with a leading --config flag so the
// root csgclaw parser can populate GlobalOptions.Config before the subcommand runs.
func commandArgsWithConfig(configPath string, args ...string) []string {
	path := strings.TrimSpace(configPath)
	if path == "" {
		return append([]string(nil), args...)
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, "--config", path)
	out = append(out, args...)
	return out
}
