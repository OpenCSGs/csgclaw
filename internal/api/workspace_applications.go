package api

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"time"
)

type workspaceOpenTarget struct {
	ID      string `json:"id"`
	Label   string `json:"label,omitempty"`
	Default bool   `json:"default,omitempty"`
}

// Use Launch Services' ranked associations, including apps outside /Applications.
// Paths are argv values, never interpolated into executable script text.
const macFileApplications = `ObjC.import('AppKit');
function run(argv) {
 const workspace = $.NSWorkspace.sharedWorkspace;
 const url = $.NSURL.fileURLWithPath(argv[0]);
 const apps = workspace.URLsForApplicationsToOpenURL(url);
 const preferred = ObjC.unwrap(workspace.URLForApplicationToOpenURL(url).path);
 const result = [];
 for (let i = 0; i < apps.count; i++) {
  const path = ObjC.unwrap(apps.objectAtIndex(i).path);
  const label = ObjC.unwrap($.NSFileManager.defaultManager.displayNameAtPath(path)).replace(/\.app$/, '');
  result.push({id: 'app:' + path, label: label, default: path === preferred});
 }
 return JSON.stringify(result);
}`

func associatedWorkspaceApplications(ctx context.Context, path string) []workspaceOpenTarget {
	if runtime.GOOS != "darwin" {
		return platformFileApplications(ctx, path)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", macFileApplications, path).Output()
	if err != nil {
		return nil
	}
	var apps []workspaceOpenTarget
	if json.Unmarshal(data, &apps) != nil {
		return nil
	}
	return apps
}
