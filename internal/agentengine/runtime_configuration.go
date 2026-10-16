package agentengine

import (
	"context"
	"errors"
)

type configurationContextKey struct{}

func (e *Engine) SetRuntimeConfigurationGuard(guard func(string) error) {
	if owner, ok := e.agents.(interface {
		SetRuntimeConfigurationGuard(func(context.Context, string) error)
	}); ok {
		owner.SetRuntimeConfigurationGuard(func(ctx context.Context, id string) error {
			if ctx.Value(configurationContextKey{}) == id || guard == nil {
				return nil
			}
			return guard(id)
		})
	}
}

func (e *Engine) UpdateRuntimeConfiguration(ctx context.Context, id string, resume bool, apply func(context.Context) error) error {
	owner, ok := e.agents.(interface {
		UpdateRuntimeConfiguration(context.Context, string, bool, func(context.Context) error) error
	})
	if !ok {
		return errors.New("runtime configuration update is unavailable")
	}
	return owner.UpdateRuntimeConfiguration(context.WithValue(ctx, configurationContextKey{}, id), id, resume, apply)
}
