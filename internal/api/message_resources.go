package api

import (
	"context"
	"errors"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/conversationresource"
	"csgclaw/internal/im"
)

func (h *Handler) prepareMessageResources(ctx context.Context, req *im.CreateMessageRequest, refs []apitypes.ResourceReference) error {
	req.Metadata = apitypes.WithResourceSelection(req.Metadata, nil)
	if len(refs) == 0 {
		if h.resources == nil || h.im == nil {
			return nil
		}
		if room, ok := h.im.Room(req.RoomID); ok && !room.IsDirect {
			return nil
		}
	}
	input, err := conversationresource.NormalizeLegacy(req.Content, refs)
	if err != nil {
		return err
	}
	if len(input.Refs) == 0 {
		return nil
	}
	if h.resources == nil || h.im == nil {
		return &conversationresource.Error{Code: conversationresource.ServiceUnavailable}
	}
	room, ok := h.im.Room(req.RoomID)
	if !ok {
		return im.ErrRoomNotFound
	}
	if !room.IsDirect || !roomHasMemberAlias(h.im, room.Members, req.SenderID) {
		return &conversationresource.Error{Code: conversationresource.InvalidRequest}
	}
	agentID := ""
	for _, member := range room.Members {
		if h.im.ResolveUserID(member) == h.im.ResolveUserID(req.SenderID) {
			continue
		}
		target := h.participantBridgeTargetForRoomMember(member)
		candidate := h.runtimeAgentIDForBridgeID(target.bridgeID)
		if candidate == "" || agentID != "" {
			return &conversationresource.Error{Code: conversationresource.InvalidRequest}
		}
		agentID = candidate
	}
	if agentID == "" {
		return &conversationresource.Error{Code: conversationresource.InvalidRequest}
	}
	resolved, err := h.resources.Resolve(ctx, agentID, input.Refs)
	if err != nil {
		var failure *conversationresource.Error
		if len(refs) == 0 && errors.As(err, &failure) && failure.Code == conversationresource.InvalidRequest {
			return nil
		}
		return err
	}
	selection := resolved.Snapshot()
	if strings.TrimSpace(input.Content) == "" && len(req.Attachments) == 0 && req.ResolveAttachments == nil && !selection.SingleSkill() {
		return &conversationresource.Error{Code: conversationresource.InvalidRequest}
	}
	req.Content = input.Content
	req.ResourceSelection = selection
	req.Metadata = apitypes.WithResourceSelection(req.Metadata, selection)
	return nil
}
