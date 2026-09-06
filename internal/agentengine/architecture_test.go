package agentengine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Keep the final dependency boundary executable, not only in architecture prose.
func TestProductionArchitectureHasNoRuntimeOrChannelBackdoors(t *testing.T) {
	for _, root := range []string{".", "../api", "../channel", "../../cli"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, item := range file.Imports {
				name, _ := strconv.Unquote(item.Path.Value)
				if name == "csgclaw/internal/agent" || strings.HasPrefix(name, "csgclaw/internal/channelbridge") {
					t.Errorf("%s imports removed owner %s", path, name)
				}
				if root == "." && (strings.HasPrefix(name, "csgclaw/internal/runtime/codex") || strings.HasPrefix(name, "csgclaw/internal/runtime/picoclaw") || strings.HasPrefix(name, "csgclaw/internal/runtime/openclaw") || strings.HasPrefix(name, "csgclaw/internal/channel/") || name == "csgclaw/internal/participant" || name == "csgclaw/internal/im") {
					t.Errorf("%s leaks concrete Runtime or Channel dependency %s", path, name)
				}
			}
			if root != "." {
				ast.Inspect(file, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					selector, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					switch selector.Sel.Name {
					case "PermissionBroker", "UserInputBroker", "PromptTurn", "EnsureEngineSession", "RespondDirect", "SessionManager":
						t.Errorf("%s calls Runtime directly: %s", path, selector.Sel.Name)
					}
					return true
				})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
