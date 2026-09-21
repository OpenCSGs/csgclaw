//go:build !windows

package dsh

import "os"

func replaceMetadataFile(source, target string) error {
	return os.Rename(source, target)
}
