package builtin

import "csgclaw/internal/skill/installstate"

func journalPath(root, name string) string { return installstate.BuiltinActivationPath(root, name) }
func writeActivation(root, name, stage, hash string) error {
	return installstate.WriteBuiltinActivation(root, name, stage, hash)
}
func recoverActivation(root, name string, record installstate.Record) (installstate.Record, bool, error) {
	return installstate.RecoverBuiltinActivation(root, name, record)
}
