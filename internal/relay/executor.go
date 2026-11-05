package relay

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type ExecutionHandler interface {
	Ready(context.Context) bool
	Execute(context.Context, Frame) Frame
}

type executionRecord struct {
	InstanceID            string `json:"instance_id"`
	InvocationFingerprint string `json:"invocation_fingerprint,omitempty"`
	Result                Frame  `json:"result"`
	Acked                 bool   `json:"acked"`
}

type executionSession struct {
	Tip       string    `json:"tip"`
	Dirty     bool      `json:"dirty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type executorState struct {
	Version  int                         `json:"version"`
	Records  map[string]executionRecord  `json:"records"`
	Sessions map[string]executionSession `json:"sessions"`
}

var errRecoveryComplete = errors.New("previous executor ownership reconciled")
var errExecutorOwnershipUnknown = errors.New("relay recovery requires the original executor instance identity; legacy unacknowledged state cannot be replayed safely")

// Executor owns a single outbound connection and one active remote invocation.
// StatePath stores the durable inbox/outbox and must be unique per publication.
type Executor struct {
	Client       *Client
	AgentID      string
	StatePath    string
	Handler      ExecutionHandler
	OnReconnect  func()
	OnConnection func(bool)
	mu           sync.Mutex
	records      map[string]executionRecord
	sessions     map[string]executionSession
	instance     string
	activeID     string
	cancel       context.CancelFunc
	fatal        error
	workers      sync.WaitGroup
}

func (e *Executor) Run(ctx context.Context) error {
	if e.Client == nil || e.Handler == nil || !safeID(e.AgentID) || e.StatePath == "" {
		return fmt.Errorf("executor requires a client, agent ID, handler and state path")
	}
	if err := os.MkdirAll(filepath.Dir(e.StatePath), 0700); err != nil {
		return err
	}
	lock, err := acquireStateLock(e.StatePath + ".lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	e.records = map[string]executionRecord{}
	e.sessions = map[string]executionSession{}
	data, err := os.ReadFile(e.StatePath)
	if err == nil {
		var state executorState
		if err = json.Unmarshal(data, &state); err != nil {
			return fmt.Errorf("invalid relay executor state")
		}
		if state.Version == 0 {
			// Legacy ACKed records can be retained for deduplication. An old
			// unacknowledged record has no recoverable executor ownership.
			if err = json.Unmarshal(data, &e.records); err != nil || e.records == nil {
				return fmt.Errorf("invalid legacy relay executor state")
			}
		} else if state.Version != 1 || state.Records == nil || state.Sessions == nil {
			return fmt.Errorf("unsupported relay executor state")
		} else {
			e.records, e.sessions = state.Records, state.Sessions
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, record := range e.records {
		if !record.Acked && !safeID(record.InstanceID) {
			return errExecutorOwnershipUnknown
		}
	}
	for id, record := range e.records {
		if record.Result.Type == "request.accepted" {
			record.Result = Frame{Type: "response.failed", ResponseID: id, Sequence: 1, Error: &ProtocolError{Code: "execution_interrupted", Message: "executor restarted; invocation was not repeated"}}
			e.records[id] = record
		}
	}
	if err = e.saveLocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer func() { cancel(); e.workers.Wait() }()
	// Restore results through their accepting instance before admitting new work.
	// A fresh runtime incarnation must never replay frames under a different owner.
	for {
		owner := ""
		for _, record := range e.records {
			if !record.Acked {
				owner = record.InstanceID
				break
			}
		}
		if owner == "" || ctx.Err() != nil {
			break
		}
		backoff := time.Second
		for ctx.Err() == nil {
			conn, connectErr := e.Client.connect(ctx, e.AgentID, owner)
			if connectErr == nil {
				connectErr = e.serve(ctx, conn, owner, true)
				conn.Close()
			}
			if errors.Is(connectErr, errRecoveryComplete) {
				break
			}
			if e.OnReconnect != nil && ctx.Err() == nil {
				e.OnReconnect()
			}
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 15*time.Second)
		}
	}
	instance := uuid.NewString()
	e.instance = instance
	backoff := time.Second
	for ctx.Err() == nil {
		e.mu.Lock()
		fatal := e.fatal
		e.mu.Unlock()
		if fatal != nil {
			return fatal
		}
		conn, err := e.Client.connect(ctx, e.AgentID, instance)
		if err == nil {
			err = e.serve(ctx, conn, instance, false)
			conn.Close()
			backoff = time.Second
		}
		if ctx.Err() != nil {
			break
		}
		if e.OnReconnect != nil {
			e.OnReconnect()
		}
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
	return nil
}

func (e *Executor) saveLocked() error {
	data, err := json.Marshal(executorState{Version: 1, Records: e.records, Sessions: e.sessions})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(e.StatePath), ".relay-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, e.StatePath)
}

func (e *Executor) serve(ctx context.Context, conn *websocket.Conn, instance string, recovering bool) error {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stop:
		}
	}()
	send := func(f Frame) error {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(f)
	}
	status := func(typ string) Frame {
		ready := false
		if !recovering {
			ready = e.Handler.Ready(ctx)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		return Frame{Type: typ, ProtocolVersion: 1, InstanceID: instance, Ready: ready && e.fatal == nil, Busy: e.activeID != ""}
	}
	if err := send(status("hello")); err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var ack Frame
	if err := conn.ReadJSON(&ack); err != nil {
		return err
	}
	if ack.Type != "hello.ack" || ack.HeartbeatIntervalSeconds <= 0 || ack.HeartbeatIntervalSeconds > 300 {
		return fmt.Errorf("invalid relay hello acknowledgement")
	}
	if e.OnConnection != nil && !recovering {
		e.OnConnection(true)
		defer e.OnConnection(false)
	}
	timeout := time.Duration(ack.HeartbeatTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(timeout)) })
	incoming := make(chan Frame)
	errorsCh := make(chan error, 1)
	go func() {
		for {
			var f Frame
			if err := conn.ReadJSON(&f); err != nil {
				errorsCh <- err
				return
			}
			select {
			case incoming <- f:
			case <-stop:
				return
			}
		}
	}()
	heartbeat := time.NewTicker(time.Duration(ack.HeartbeatIntervalSeconds) * time.Second)
	defer heartbeat.Stop()
	retry := time.NewTicker(time.Second)
	defer retry.Stop()
	recovered := func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		for _, record := range e.records {
			if !record.Acked && record.InstanceID == instance {
				return false
			}
		}
		return true
	}
	flush := func() error {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.fatal != nil {
			return e.fatal
		}
		for _, record := range e.records {
			if !record.Acked && record.InstanceID == instance && record.Result.Type != "request.accepted" {
				if err := send(record.Result); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := flush(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errorsCh:
			return err
		case <-heartbeat.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return err
			}
			if err := send(status("heartbeat")); err != nil {
				return err
			}
		case <-retry.C:
			if err := flush(); err != nil {
				return err
			}
		case frame := <-incoming:
			switch frame.Type {
			case "request":
				if recovering {
					return fmt.Errorf("relay dispatched new work to a recovery-only connection")
				}
				if err := e.accept(ctx, frame, send); err != nil {
					return err
				}
			case "request.cancel":
				e.mu.Lock()
				if e.activeID == frame.ResponseID && e.cancel != nil {
					e.cancel()
				}
				e.mu.Unlock()
			case "result.ack":
				e.mu.Lock()
				if record, ok := e.records[frame.ResponseID]; ok && record.InstanceID == instance && record.Result.Type != "request.accepted" {
					record.Acked = true
					e.records[frame.ResponseID] = record
					e.fatal = e.saveLocked()
				}
				fatal := e.fatal
				e.mu.Unlock()
				if fatal != nil {
					return fatal
				}
				if recovering && recovered() {
					return errRecoveryComplete
				}
			}
		}
	}
}

func (e *Executor) accept(ctx context.Context, frame Frame, send func(Frame) error) error {
	if !safeID(frame.ResponseID) || !safeID(frame.SessionID) || (frame.PreviousResponseID != "" && !safeID(frame.PreviousResponseID)) || frame.Deadline.IsZero() {
		return fmt.Errorf("invalid relay request identity or deadline")
	}
	input, err := InputText(frame.Input)
	if err != nil {
		return fmt.Errorf("invalid relay request input")
	}
	// Relay response IDs can repeat after the idempotency record expires.
	// The original deadline is stable across redeliveries of one admission.
	identity, _ := json.Marshal([]string{frame.SessionID, frame.PreviousResponseID, frame.Deadline.UTC().Format(time.RFC3339Nano), input})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(identity))
	record := executionRecord{InstanceID: e.instance, InvocationFingerprint: fingerprint}
	ready := e.Handler.Ready(ctx)
	e.mu.Lock()
	if old, ok := e.records[frame.ResponseID]; ok {
		if old.InstanceID == record.InstanceID && old.InvocationFingerprint == fingerprint {
			e.mu.Unlock()
			return send(old.Result)
		}
		// The current admission cannot use the cached result. Persist the
		// rejection so the outbox cannot replay the old result for this ID.
		record.Result = Frame{Type: "response.failed", ResponseID: frame.ResponseID, Sequence: 1, Error: &ProtocolError{
			Code: "relay_invocation_identity_mismatch", Message: "response ID belongs to a different or unverifiable invocation; use a new idempotency key",
		}}
		e.records[frame.ResponseID] = record
		err := e.saveLocked()
		if err != nil {
			e.fatal = err
		}
		e.mu.Unlock()
		if err != nil {
			return err
		}
		return send(record.Result)
	}
	// Reclaim only ACKed entries. Unacknowledged results are never evicted.
	if len(e.records) >= 1024 {
		for id, record := range e.records {
			if record.Acked {
				delete(e.records, id)
				break
			}
		}
	}
	if len(e.records) >= 1024 {
		e.mu.Unlock()
		return fmt.Errorf("relay outbox is full")
	}
	session := e.sessions[frame.SessionID]
	if session.Dirty || session.Tip != frame.PreviousResponseID {
		result := Frame{Type: "response.failed", ResponseID: frame.ResponseID, Sequence: 1, Error: &ProtocolError{Code: "relay_branch_unsupported", Message: "continuation does not match the confirmed local session tip; start a new conversation"}}
		record.Result = result
		e.records[frame.ResponseID] = record
		e.fatal = e.saveLocked()
		fatal := e.fatal
		e.mu.Unlock()
		if fatal != nil {
			return fatal
		}
		return send(result)
	}
	if !ready || e.activeID != "" {
		result := Frame{Type: "response.failed", ResponseID: frame.ResponseID, Sequence: 1, Error: &ProtocolError{Code: "agent_unavailable", Message: "local executor is not ready"}}
		if e.activeID != "" {
			result.Error = &ProtocolError{Code: "agent_busy", Message: "executor already has an active invocation"}
		}
		record.Result = result
		e.records[frame.ResponseID] = record
		e.fatal = e.saveLocked()
		fatal := e.fatal
		e.mu.Unlock()
		if fatal != nil {
			return fatal
		}
		return send(result)
	}
	record.Result = Frame{Type: "request.accepted", ResponseID: frame.ResponseID}
	e.records[frame.ResponseID] = record
	frame.parentValidated = true
	if _, exists := e.sessions[frame.SessionID]; !exists && len(e.sessions) >= 1024 {
		oldest := ""
		for id, cached := range e.sessions {
			if oldest == "" || cached.UpdatedAt.Before(e.sessions[oldest].UpdatedAt) {
				oldest = id
			}
		}
		delete(e.sessions, oldest)
	}
	session.UpdatedAt = time.Now().UTC()
	session.Dirty = true
	e.sessions[frame.SessionID] = session
	if err := e.saveLocked(); err != nil {
		e.fatal = err
		e.mu.Unlock()
		return err
	}
	deadline := frame.Deadline
	if max := time.Now().Add(5 * time.Minute); deadline.After(max) {
		deadline = max
	}
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	e.activeID = frame.ResponseID
	e.cancel = cancel
	e.mu.Unlock()
	// Persist and start even if the acceptance write fails: relay does not redispatch
	// on acceptance timeout and this result can be delivered after reconnection.
	e.workers.Add(1)
	go func() {
		defer e.workers.Done()
		defer cancel()
		result := Frame{Type: "response.cancelled", ResponseID: frame.ResponseID, Sequence: 1}
		if runCtx.Err() == nil {
			result = e.Handler.Execute(runCtx, frame)
		}
		result.ResponseID = frame.ResponseID
		result.Sequence = 1
		if result.Type != "response.completed" && result.Type != "response.failed" && result.Type != "response.cancelled" {
			result = Frame{Type: "response.failed", ResponseID: frame.ResponseID, Sequence: 1, Error: &ProtocolError{Code: "execution_failed", Message: "invalid executor result"}}
		}
		if data, _ := json.Marshal(result); len(data) > 2<<20 {
			result = Frame{Type: "response.failed", ResponseID: frame.ResponseID, Sequence: 1, Error: &ProtocolError{Code: "output_too_large", Message: "executor result exceeds frame limit"}}
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		e.activeID = ""
		e.cancel = nil
		current := e.records[frame.ResponseID]
		if current.InvocationFingerprint != fingerprint || current.Result.Type != "request.accepted" {
			// A reused ID superseded this admission while it was executing.
			// Leave its session uncertain and preserve the new rejection.
			return
		}
		record.Result = result
		e.records[frame.ResponseID] = record
		if result.Type == "response.completed" {
			e.sessions[frame.SessionID] = executionSession{Tip: frame.ResponseID, UpdatedAt: time.Now().UTC()}
		}
		e.fatal = e.saveLocked()
	}()
	return send(Frame{Type: "request.accepted", ResponseID: frame.ResponseID})
}
