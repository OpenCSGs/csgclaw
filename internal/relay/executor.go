package relay

import (
	"context"
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
	Result Frame `json:"result"`
	Acked  bool  `json:"acked"`
}

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
	data, err := os.ReadFile(e.StatePath)
	if err == nil {
		if err = json.Unmarshal(data, &e.records); err != nil || e.records == nil {
			return fmt.Errorf("invalid relay executor state")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
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
	instance := uuid.NewString()
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
			err = e.serve(ctx, conn, instance)
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
	data, err := json.Marshal(e.records)
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

func (e *Executor) serve(ctx context.Context, conn *websocket.Conn, instance string) error {
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
		ready := e.Handler.Ready(ctx)
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
	if e.OnConnection != nil {
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
	flush := func() error {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.fatal != nil {
			return e.fatal
		}
		for _, record := range e.records {
			if !record.Acked && record.Result.Type != "request.accepted" {
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
				if record, ok := e.records[frame.ResponseID]; ok && record.Result.Type != "request.accepted" {
					record.Acked = true
					e.records[frame.ResponseID] = record
					e.fatal = e.saveLocked()
				}
				fatal := e.fatal
				e.mu.Unlock()
				if fatal != nil {
					return fatal
				}
			}
		}
	}
}

func (e *Executor) accept(ctx context.Context, frame Frame, send func(Frame) error) error {
	if !safeID(frame.ResponseID) || !safeID(frame.SessionID) || frame.Deadline.IsZero() {
		return fmt.Errorf("invalid relay request identity or deadline")
	}
	if _, err := InputText(frame.Input); err != nil {
		return fmt.Errorf("invalid relay request input")
	}
	ready := e.Handler.Ready(ctx)
	e.mu.Lock()
	if old, ok := e.records[frame.ResponseID]; ok {
		e.mu.Unlock()
		return send(old.Result)
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
	if !ready || e.activeID != "" {
		result := Frame{Type: "response.failed", ResponseID: frame.ResponseID, Sequence: 1, Error: &ProtocolError{Code: "agent_unavailable", Message: "local executor is not ready"}}
		if e.activeID != "" {
			result.Error = &ProtocolError{Code: "agent_busy", Message: "executor already has an active invocation"}
		}
		e.records[frame.ResponseID] = executionRecord{Result: result}
		e.fatal = e.saveLocked()
		fatal := e.fatal
		e.mu.Unlock()
		if fatal != nil {
			return fatal
		}
		return send(result)
	}
	e.records[frame.ResponseID] = executionRecord{Result: Frame{Type: "request.accepted", ResponseID: frame.ResponseID}}
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
		e.records[frame.ResponseID] = executionRecord{Result: result}
		e.activeID = ""
		e.cancel = nil
		e.fatal = e.saveLocked()
	}()
	return send(Frame{Type: "request.accepted", ResponseID: frame.ResponseID})
}
