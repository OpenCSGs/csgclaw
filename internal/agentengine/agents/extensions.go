package agents

import "context"

// RuntimeExtensionsController prepares child resources under the Agent mutation
// lease before Runtime startup; Channel lifetimes are not involved.
type RuntimeExtensionsController interface {
	PrepareRuntime(context.Context, string) error
	RuntimeStarted(context.Context, string) error
	RuntimeStopped(context.Context, string) error
	DeleteExtensions(context.Context, string) error
	RuntimeReady(string) error
}

func (s *Controller) prepareExtensions(ctx context.Context, id string) error {
	if s.extensions == nil {
		return nil
	}
	if _, exists := s.agentSnapshot(id); !exists {
		return nil
	}
	return s.extensions.PrepareRuntime(ctx, id)
}
func (s *Controller) observeStartedExtensions(ctx context.Context, id string) error {
	if s.extensions == nil {
		return nil
	}
	return s.extensions.RuntimeStarted(ctx, id)
}
func (s *Controller) observeStoppedExtensions(ctx context.Context, id string) error {
	if s.interactions != nil {
		s.interactions.Interrupt(id, "", "", true)
	}
	if s.extensions == nil {
		return nil
	}
	return s.extensions.RuntimeStopped(ctx, id)
}
