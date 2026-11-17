package api

import "net/http"

// The actor comes from authenticated caller context or the local workspace
// identity, never from user-controlled Agent fields in a JSON body.
func (h *Handler) agentMutationActor(r *http.Request) string {
	if caller := appRequestAgentID(r); caller != "" {
		return caller
	}
	if h.im != nil {
		return h.im.CurrentUserID()
	}
	return ""
}
