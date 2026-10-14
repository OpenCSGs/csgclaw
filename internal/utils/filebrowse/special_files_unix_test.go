//go:build unix

package filebrowse

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWorkspaceFIFOIsRejectedWithoutBlocking(t *testing.T) {
	if root := os.Getenv("CSGCLAW_FIFO_TEST_ROOT"); root != "" {
		file, err := OpenFile(root, "pipe")
		if err == nil {
			file.Close()
			t.Fatal("accepted a FIFO")
		}
		return
	}
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkspaceFIFOIsRejectedWithoutBlocking$")
	command.Env = append(os.Environ(), "CSGCLAW_FIFO_TEST_ROOT="+root)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("opening a FIFO without a writer blocked before rejection")
	}
	if err != nil {
		t.Fatalf("FIFO rejection: %v %s", err, output)
	}
}

func TestWorkspaceReplacementCannotBlockOpenOrTraversal(t *testing.T) {
	if root := os.Getenv("CSGCLAW_REPLACEMENT_TEST_ROOT"); root != "" {
		bounded, err := os.OpenRoot(root)
		if err != nil {
			t.Fatal(err)
		}
		defer bounded.Close()
		info, err := bounded.Lstat("target")
		if err != nil || !info.Mode().IsRegular() {
			t.Fatal("regular-file control is missing")
		}
		if err := os.Remove(filepath.Join(root, "target")); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(filepath.Join(root, "target"), 0600); err != nil {
			t.Fatal(err)
		}
		file, err := openNonblocking(bounded, "target")
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
		if _, err := openBoundedPath(bounded, "target", false); err == nil {
			t.Fatal("replacement FIFO accepted")
		}
		err = fs.WalkDir(safeDirectoryFS{root: bounded}, "docs", func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if name == "docs" {
				if !entry.IsDir() {
					t.Fatal("directory control is missing")
				}
				if err := os.Remove(filepath.Join(root, "docs")); err != nil {
					return err
				}
				return syscall.Mkfifo(filepath.Join(root, "docs"), 0600)
			}
			return nil
		})
		if err == nil {
			t.Fatal("directory replacement was accepted")
		}
		return
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target"), []byte("control"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkspaceReplacementCannotBlockOpenOrTraversal$")
	command.Env = append(os.Environ(), "CSGCLAW_REPLACEMENT_TEST_ROOT="+root)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("replacement FIFO blocked file open or directory traversal")
	}
	if err != nil {
		t.Fatalf("replacement rejection: %v %s", err, output)
	}
}

func TestWorkspaceListingOmitsSpecialFiles(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, list := range map[string]func(string, string) (bool, error){
		"directory": func(root, path string) (bool, error) {
			listing, err := ListDirectory(root, path)
			return len(listing.Entries) == 0, err
		},
		"recursive": func(root, path string) (bool, error) {
			listing, err := List(root, path)
			return len(listing.Entries) == 0, err
		},
	} {
		t.Run(name, func(t *testing.T) {
			empty, err := list(root, "")
			if err != nil || !empty {
				t.Fatalf("special file listed: empty=%t, error=%v", empty, err)
			}
		})
	}
}
