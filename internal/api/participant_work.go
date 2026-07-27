package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/worklease"
)

const (
	participantWorkStatusBodyLimit = 32 * 1024
	participantTurnStopTimeout     = 10 * time.Second
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

func (h *Handler) patchParticipantWorkLease(w http.ResponseWriter, r *http.Request) {
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
	controller, ok := h.participantWork.(worklease.ParticipantWorkController)
	if !ok {
		http.Error(w, "participant work leases are not configured", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, participantWorkStatusBodyLimit)
	var request apitypes.ParticipantWorkStatusPatchRequest
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
	update, accepted, err := controller.UpdateStatus(r.Context(), participantID, leaseID, request)
	if err != nil {
		h.writeParticipantWorkError(w, err)
		return
	}
	if !accepted {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, update)
}

func (h *Handler) stopParticipantWork(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(chi.URLParam(r, "channel")) != "csgclaw" {
		http.NotFound(w, r)
		return
	}
	participantID := strings.TrimSpace(chi.URLParam(r, "id"))
	if participantID == "" {
		http.Error(w, "invalid participant id", http.StatusBadRequest)
		return
	}
	controller, ok := h.participantWork.(worklease.ParticipantWorkController)
	if !ok {
		http.Error(w, "participant work controls are not configured", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, participantWorkStatusBodyLimit)
	var request apitypes.ParticipantWorkStopRequest
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
	request.LeaseID = strings.TrimSpace(request.LeaseID)
	request.RequestID = strings.TrimSpace(request.RequestID)
	if request.RoomID == "" || request.RequestID == "" || !worklease.ValidID(request.LeaseID) {
		http.Error(w, "room_id, request_id, and a valid lease_id are required", http.StatusBadRequest)
		return
	}
	stopCtx, cancel := context.WithTimeout(r.Context(), participantTurnStopTimeout)
	defer cancel()
	response, err := controller.RequestStop(stopCtx, participantID, request)
	if err != nil {
		h.writeParticipantWorkError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, response)
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
	outcome := apitypes.ParticipantWorkOutcomeReleased
	r.Body = http.MaxBytesReader(w, r.Body, participantWorkStatusBodyLimit)
	var request apitypes.ParticipantWorkReleaseRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	} else if err == nil {
		if err := ensureJSONEOF(decoder); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if value := strings.TrimSpace(request.Outcome); value != "" {
			outcome = value
		}
	}
	var err error
	if finisher, ok := h.participantWork.(worklease.ParticipantWorkFinisher); ok {
		err = finisher.Finish(r.Context(), participantID, leaseID, outcome)
	} else {
		err = h.participantWork.Stop(r.Context(), participantID, leaseID)
	}
	if err != nil {
		h.writeParticipantWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) writeParticipantWorkError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, worklease.ErrParticipantNotFound), errors.Is(err, worklease.ErrRoomNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, worklease.ErrLeaseNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, worklease.ErrNotRoomMember):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, worklease.ErrConflict):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, worklease.ErrInvalidStatus):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, worklease.ErrRateLimited):
		w.Header().Set("Retry-After", "1")
		http.Error(w, err.Error(), http.StatusTooManyRequests)
	case errors.Is(err, worklease.ErrTurnControlTimedOut):
		http.Error(w, err.Error(), http.StatusGatewayTimeout)
	case errors.Is(err, worklease.ErrTurnControlFailed):
		http.Error(w, err.Error(), http.StatusBadGateway)
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
