package browser

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"
)

const ControlHeader = "X-CSGClaw-Browser-Control"
const FrameHeader = "X-CSGClaw-Browser-Frame"
const controlLifetime = 30 * time.Second

var ErrUserControl = errors.New("browser is controlled by a user; wait until control is released")
var ErrControlRequired = errors.New("take over the browser and load its current frame before sending input")

type UserControl struct {
	Session State  `json:"session"`
	Token   string `json:"token"`
}

func (s *Service) userControlledLocked(e *session) bool {
	return e.controlToken != "" && time.Now().Before(e.controlUntil)
}
func (s *Service) stateLocked(e *session) State {
	state := e.state
	state.UserControlled = s.userControlledLocked(e)
	return state
}
func (s *Service) checkControl(e *session, token string, required bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := s.userControlledLocked(e)
	if token != "" {
		if !active || subtle.ConstantTimeCompare([]byte(token), []byte(e.controlToken)) != 1 {
			return ErrControlRequired
		}
		e.controlUntil = time.Now().Add(controlLifetime)
		return nil
	}
	if required {
		return ErrControlRequired
	}
	if active {
		return ErrUserControl
	}
	return nil
}
func (s *Service) TakeControl(ctx context.Context, room, id string) (UserControl, error) {
	e, release, err := s.acquire(ctx, room, id)
	if err != nil {
		return UserControl{}, err
	}
	defer release()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userControlledLocked(e) {
		return UserControl{}, ErrUserControl
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return UserControl{}, err
	}
	e.controlToken = hex.EncodeToString(raw)
	e.controlUntil = time.Now().Add(controlLifetime)
	e.controlFrameReady = false
	e.controlRevision++
	return UserControl{Session: s.stateLocked(e), Token: e.controlToken}, nil
}
func (s *Service) ReleaseControl(ctx context.Context, room, id, token string) (State, error) {
	e, release, err := s.acquire(ctx, room, id)
	if err != nil {
		return State{}, err
	}
	defer release()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userControlledLocked(e) && (token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(e.controlToken)) != 1) {
		return State{}, ErrControlRequired
	}
	e.controlToken = ""
	e.controlUntil = time.Time{}
	return s.stateLocked(e), nil
}
func (s *Service) RunUser(ctx context.Context, room, id string, command Command, token string) (State, error) {
	return s.run(ctx, room, id, command, token, true)
}
func (s *Service) CloseAgentSession(ctx context.Context, room, id string) error {
	return s.closeSessionWithControl(ctx, room, id, time.Time{}, true)
}
func (s *Service) UserScreenshot(ctx context.Context, room, id string, full bool, token string) (Frame, error) {
	return s.capture(ctx, room, id, full, token)
}
