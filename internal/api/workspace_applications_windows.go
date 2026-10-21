package api

import (
	"context"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var workspaceAssocQueryString = windows.NewLazySystemDLL("shlwapi.dll").NewProc("AssocQueryStringW")

func platformFileApplications(_ context.Context, path string) []workspaceOpenTarget {
	extension, err := windows.UTF16PtrFromString(filepath.Ext(path))
	if err != nil || filepath.Ext(path) == "" {
		return nil
	}
	query := func(kind uintptr) string {
		size := uint32(32768)
		buffer := make([]uint16, size)
		result, _, _ := workspaceAssocQueryString.Call(0, kind, uintptr(unsafe.Pointer(extension)), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
		if result != 0 {
			return ""
		}
		return syscall.UTF16ToString(buffer)
	}
	// ASSOCSTR_EXECUTABLE and ASSOCSTR_FRIENDLYAPPNAME respect the user's file association.
	executable, name := query(2), query(4)
	if executable == "" {
		return nil
	}
	if name == "" {
		name = filepath.Base(executable)
	}
	return []workspaceOpenTarget{{ID: "default", Label: name, Default: true}}
}
