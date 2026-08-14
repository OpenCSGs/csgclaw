package files

import (
	"context"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge"
)

// Resolver authorizes one source attachment and resolves it to an Engine InputFile.
// The returned release function must keep the source valid until the turn finishes.
type Resolver interface {
	Resolve(
		ctx context.Context,
		binding channel.Binding,
		event channelbridge.BotEvent,
		attachment channelbridge.MessageAttachment,
	) (file agentengine.InputFile, release func(), err error)
}

// ContextResolver rematerializes hidden thread-context attachments for the
// current source message. This avoids queued replies sharing a workspace path
// that an earlier turn may release before the later turn starts.
type ContextResolver interface {
	ResolveContext(
		ctx context.Context,
		binding channel.Binding,
		event channelbridge.BotEvent,
	) (resolved channelbridge.BotEvent, release func(), err error)
}
