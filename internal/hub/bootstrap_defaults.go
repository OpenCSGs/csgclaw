package hub

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/runtime"
)

type BootstrapDefaults struct {
	ManagerTemplateRef string
	ManagerRuntimeKind string
	ManagerImage       string
	WorkerTemplateRef  string
	WorkerRuntimeKind  string
	WorkerImage        string
}

func ResolveBootstrapDefaults(ctx context.Context, bootstrap config.BootstrapConfig, svc *Service) (BootstrapDefaults, error) {
	defaults := BootstrapDefaults{
		ManagerTemplateRef: bootstrap.ResolvedDefaultManagerTemplate(),
		WorkerTemplateRef:  bootstrap.ResolvedDefaultWorkerTemplate(),
	}
	if svc == nil {
		return BootstrapDefaults{}, fmt.Errorf("hub service is required to resolve bootstrap templates")
	}

	manager, err := svc.Get(ctx, defaults.ManagerTemplateRef)
	if err != nil {
		return BootstrapDefaults{}, fmt.Errorf("resolve bootstrap manager template %q: %w", defaults.ManagerTemplateRef, err)
	}
	defaults.ManagerRuntimeKind = manager.RuntimeKind
	if defaults.ManagerRuntimeKind != runtime.KindPicoClawSandbox {
		return BootstrapDefaults{}, fmt.Errorf("bootstrap manager template %q uses unsupported runtime_kind %q (use %q)", defaults.ManagerTemplateRef, manager.RuntimeKind, runtime.KindPicoClawSandbox)
	}
	defaults.ManagerImage = templateImageOrDefault(defaults.ManagerRuntimeKind, manager.Image)

	worker, err := svc.Get(ctx, defaults.WorkerTemplateRef)
	if err != nil {
		return BootstrapDefaults{}, fmt.Errorf("resolve bootstrap worker template %q: %w", defaults.WorkerTemplateRef, err)
	}
	defaults.WorkerRuntimeKind = worker.RuntimeKind
	if defaults.WorkerRuntimeKind == "" {
		return BootstrapDefaults{}, fmt.Errorf("bootstrap worker template %q uses unsupported runtime_kind %q", defaults.WorkerTemplateRef, worker.RuntimeKind)
	}
	defaults.WorkerImage = templateImageOrDefault(defaults.WorkerRuntimeKind, worker.Image)

	return defaults, nil
}

func templateImageOrDefault(runtimeKind, image string) string {
	if image = strings.TrimSpace(image); image != "" {
		return image
	}
	switch runtimeKind {
	case runtime.KindPicoClawSandbox, runtime.KindOpenClawSandbox:
		return config.DefaultManagerImageForRuntimeKind(runtimeKind)
	default:
		return ""
	}
}
