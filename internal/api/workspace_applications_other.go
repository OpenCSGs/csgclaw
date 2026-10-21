//go:build !windows && !linux

package api

import "context"

func platformFileApplications(context.Context, string) []workspaceOpenTarget { return nil }
