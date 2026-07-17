//go:build linux

package codex

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"
)

func runtimeDirFilesystemType(path string) (string, string) {
	probe := filepath.Clean(path)
	for {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(probe, &stat); err == nil {
			return linuxFilesystemType(stat.Type), ""
		} else if !errors.Is(err, syscall.ENOENT) {
			return "unknown", err.Error()
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "unknown", syscall.ENOENT.Error()
		}
		probe = parent
	}
}

func linuxFilesystemType(magic int64) string {
	switch magic {
	case 0x6969:
		return "nfs"
	case 0xff534d42:
		return "cifs"
	case 0xfe534d42:
		return "smb2"
	case 0x65735546:
		return "fuse"
	case 0x794c7630:
		return "overlayfs"
	case 0x01021994:
		return "tmpfs"
	case 0xef53:
		return "ext2/3/4"
	case 0x58465342:
		return "xfs"
	case 0x9123683e:
		return "btrfs"
	case 0x2fc12fc1:
		return "zfs"
	default:
		return fmt.Sprintf("unknown(0x%x)", uint64(magic))
	}
}
