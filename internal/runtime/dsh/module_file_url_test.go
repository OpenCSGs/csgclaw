package dsh

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	agentruntime "csgclaw/internal/runtime"

	"gopkg.in/yaml.v3"
)

func TestManagedDSHModulesLoadFromPatchURLs(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is unavailable")
	}
	root := filepath.Join(t.TempDir(), "安装 100% #")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimePatch(filepath.Join(root, patchFileName), agentruntime.Profile{}); err != nil {
		t.Fatal(err)
	}
	if err := writeMemoryPatch(root, nil, agentruntime.Profile{}); err != nil {
		t.Fatal(err)
	}
	var modules []string
	for _, name := range []string{contextPatchFileName, memoryPatchFileName} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var patch []struct {
			Insert []struct{ Name string }
		}
		if err := yaml.Unmarshal(data, &patch); err != nil {
			t.Fatal(err)
		}
		for _, block := range patch {
			for _, plugin := range block.Insert {
				modules = append(modules, plugin.Name)
			}
		}
	}
	if len(modules) != 2 {
		t.Fatalf("managed modules = %q", modules)
	}
	// Exercise Node's real ESM resolver using exactly the names DSH will load.
	script := `for (const url of process.argv.slice(1)) {
  if (!url.startsWith('file:')) throw new Error('Expected a file URL: ' + url);
  const module = await import(url);
  if (typeof module.apply !== 'function') throw new Error('Invalid DSH plugin');
}`
	args := append([]string{"--input-type=module", "-e", script}, modules...)
	if out, err := exec.Command(node, args...).CombinedOutput(); err != nil {
		t.Fatalf("load managed modules: %v\n%s", err, out)
	}
}
