package dshcli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type httpDoFunc func(*http.Request) (*http.Response, error)

func (f httpDoFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestInstallManagedNodeDownloadsAndVerifiesOfficialArchiveShape(t *testing.T) {
	const filename = "node-v24.21.0-linux-x64.tar.gz"
	archive := testNodeArchive(t, map[string]struct {
		mode    int64
		content string
	}{
		"node-v24.21.0-linux-x64/bin/node":                                       {mode: 0o755, content: "node"},
		"node-v24.21.0-linux-x64/lib/node_modules/npm/bin/npm-cli.js":            {mode: 0o644, content: "npm"},
		"node-v24.21.0-linux-x64/lib/node_modules/npm/package.json":              {mode: 0o644, content: "{}"},
		"node-v24.21.0-linux-x64/lib/node_modules/npm/node_modules/.placeholder": {mode: 0o644, content: ""},
	})
	checksum := sha256.Sum256(archive)
	manifest := []byte(fmt.Sprintf("%x  %s\n", checksum, filename))
	client := httpDoFunc(func(request *http.Request) (*http.Response, error) {
		var body []byte
		switch filepath.Base(request.URL.Path) {
		case "SHASUMS256.txt":
			body = manifest
		case filename:
			body = archive
		default:
			return nil, fmt.Errorf("unexpected URL %s", request.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})
	installRoot := t.TempDir()
	installer := Installer{
		HTTPClient:     client,
		NodeReleaseURL: "https://node.test/latest-v24.x",
		GOOS:           "linux",
		GOARCH:         "amd64",
		Run: func(_ context.Context, path string, args ...string) ([]byte, error) {
			if filepath.Base(path) != "node" || len(args) != 1 || args[0] != "--version" {
				return nil, fmt.Errorf("unexpected command %s %v", path, args)
			}
			return []byte("v24.21.0"), nil
		},
	}
	runtime, err := installer.installManagedNode(context.Background(), installRoot, nil)
	if err != nil {
		t.Fatalf("installManagedNode() error = %v", err)
	}
	if runtime.NodePath != filepath.Join(installRoot, "node-v24.21.0-linux-amd64", "bin", "node") ||
		runtime.NPMCLI != filepath.Join(installRoot, "node-v24.21.0-linux-amd64", "lib", "node_modules", "npm", "bin", "npm-cli.js") {
		t.Fatalf("managed runtime = %+v", runtime)
	}
	for _, path := range []string{runtime.NodePath, runtime.NPMCLI} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("managed runtime file %s: %v", path, err)
		}
	}
}

func TestInstallManagedNodePrefersDomesticMirrorAndFallsBackToOfficial(t *testing.T) {
	const filename = "node-v24.21.0-linux-x64.tar.gz"
	archive := testNodeArchive(t, map[string]struct {
		mode    int64
		content string
	}{
		"node-v24.21.0-linux-x64/bin/node":                            {mode: 0o755, content: "node"},
		"node-v24.21.0-linux-x64/lib/node_modules/npm/bin/npm-cli.js": {mode: 0o644, content: "npm"},
	})
	checksum := sha256.Sum256(archive)
	manifest := []byte(fmt.Sprintf("%x  %s\n", checksum, filename))
	var requests []string
	client := httpDoFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL.String())
		if strings.HasPrefix(request.URL.String(), DefaultNodeMirrorURL+"/") {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
				Body:       io.NopCloser(strings.NewReader("mirror unavailable")),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}
		var body []byte
		switch filepath.Base(request.URL.Path) {
		case "SHASUMS256.txt":
			body = manifest
		case filename:
			body = archive
		default:
			return nil, fmt.Errorf("unexpected URL %s", request.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})
	installer := Installer{
		HTTPClient: client,
		GOOS:       "linux",
		GOARCH:     "amd64",
		Run: func(_ context.Context, path string, args ...string) ([]byte, error) {
			if filepath.Base(path) != "node" || len(args) != 1 || args[0] != "--version" {
				return nil, fmt.Errorf("unexpected command %s %v", path, args)
			}
			return []byte("v24.21.0"), nil
		},
	}
	managed, err := installer.installManagedNode(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("installManagedNode() error = %v", err)
	}
	if managed.Version != "" {
		t.Fatalf("managed runtime version = %q, want validation to remain external", managed.Version)
	}
	wantRequests := []string{
		DefaultNodeMirrorURL + "/SHASUMS256.txt",
		OfficialNodeReleaseURL + "/SHASUMS256.txt",
		OfficialNodeReleaseURL + "/" + filename,
	}
	if len(requests) != len(wantRequests) {
		t.Fatalf("requests = %q, want %q", requests, wantRequests)
	}
	for index := range wantRequests {
		if requests[index] != wantRequests[index] {
			t.Fatalf("requests[%d] = %q, want %q", index, requests[index], wantRequests[index])
		}
	}
}

func TestDownloadNodeArchiveRejectsChecksumMismatch(t *testing.T) {
	client := httpDoFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(bytes.NewReader([]byte("not the expected archive"))),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})
	err := (Installer{HTTPClient: client}).downloadNodeArchive(
		context.Background(),
		"https://node.test/node.tar.gz",
		filepath.Join(t.TempDir(), "node.tar.gz"),
		sha256.Sum256([]byte("expected")),
	)
	if err == nil {
		t.Fatal("downloadNodeArchive() error = nil, want checksum mismatch")
	}
}

func testNodeArchive(t *testing.T, files map[string]struct {
	mode    int64
	content string
}) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, file := range files {
		contents := []byte(file.content)
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: file.mode, Size: int64(len(contents)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
