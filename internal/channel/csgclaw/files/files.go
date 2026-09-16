package files

import (
	"context"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
)

// Resolver authorizes one source attachment and resolves it to an Engine InputFile.
// The returned release function must keep the source valid until the turn finishes.
type Resolver interface {
	Resolve(
		ctx context.Context,
		binding channel.Binding,
		event channel.Event,
		attachment channel.MessageAttachment,
	) (file agentengine.InputFile, release func(), err error)
}

// EventAttachments collects current and inherited files in a stable order.
// Repeated references to the same published attachment produce one native input.
func EventAttachments(event channel.Event) []channel.MessageAttachment {
	var out []channel.MessageAttachment
	seen := make(map[string]bool)
	add := func(items []channel.MessageAttachment) {
		for _, item := range items {
			if item.ID != "" && seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			out = append(out, item)
		}
	}
	add(event.Attachments)
	if event.ThreadContext != nil {
		for _, message := range event.ThreadContext.Context {
			add(message.Attachments)
		}
	}
	return out
}
