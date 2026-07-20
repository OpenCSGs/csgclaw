package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/worklease"
)

func (h *Handler) putParticipantWorkLease(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(chi.URLParam(r, "channel")) != "csgclaw" {
		http.NotFound(w, r)
		return
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	participantID := strings.TrimSpace(chi.URLParam(r, "id"))
	leaseID := strings.TrimSpace(chi.URLParam(r, "lease_id"))
	if participantID == "" || !worklease.ValidID(leaseID) {
		http.Error(w, "invalid participant or lease id", http.StatusBadRequest)
		return
	}

	var request apitypes.ParticipantWorkLeaseRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	request.RoomID = strings.TrimSpace(request.RoomID)
	request.ThreadRootID = strings.TrimSpace(request.ThreadRootID)
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.Kind = strings.TrimSpace(request.Kind)
	if request.RoomID == "" || request.RequestID == "" || request.Kind != apitypes.ParticipantWorkKindAgentTurn {
		http.Error(w, "room_id and request_id are required and kind must be agent_turn", http.StatusBadRequest)
		return
	}
	if h.participantWork == nil {
		http.Error(w, "participant work leases are not configured", http.StatusServiceUnavailable)
		return
	}

	ttl := 0
	ttlExplicit := request.TTLSeconds != nil
	if ttlExplicit {
		ttl = *request.TTLSeconds
	}
	update, err := h.participantWork.StartOrRenew(r.Context(), worklease.ParticipantWorkLease{
		ParticipantID: participantID,
		LeaseID:       leaseID,
		RoomID:        request.RoomID,
		ThreadRootID:  request.ThreadRootID,
		RequestID:     request.RequestID,
		Kind:          request.Kind,
		TTLSeconds:    ttl,
		TTLExplicit:   ttlExplicit,
	})
	if err != nil {
		h.writeParticipantWorkError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, update)
}

func (h *Handler) deleteParticipantWorkLease(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(chi.URLParam(r, "channel")) != "csgclaw" {
		http.NotFound(w, r)
		return
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	participantID := strings.TrimSpace(chi.URLParam(r, "id"))
	leaseID := strings.TrimSpace(chi.URLParam(r, "lease_id"))
	if participantID == "" || !worklease.ValidID(leaseID) {
		http.Error(w, "invalid participant or lease id", http.StatusBadRequest)
		return
	}
	if h.participantWork == nil {
		http.Error(w, "participant work leases are not configured", http.StatusServiceUnavailable)
		return
	}
	if err := h.participantWork.Stop(r.Context(), participantID, leaseID); err != nil && !errors.Is(err, worklease.ErrUnavailable) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if errors.Is(err, worklease.ErrUnavailable) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) writeParticipantWorkError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, worklease.ErrParticipantNotFound), errors.Is(err, worklease.ErrRoomNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, worklease.ErrNotRoomMember):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, worklease.ErrConflict):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, worklease.ErrClosed):
		epoch := ""
		if source, ok := h.participantWork.(interface{ Epoch() string }); ok {
			epoch = source.Epoch()
		}
		writeJSON(w, http.StatusGone, apitypes.ParticipantWorkClosedResponse{
			Error:         worklease.ErrClosed.Error(),
			RegistryEpoch: epoch,
		})
	case errors.Is(err, worklease.ErrUnavailable):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	return fmt.Errorf("decode request: multiple JSON values")
}
