package apps

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/localstore"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const installationsSection = "app_installations"

type record struct {
	Installation
	Package          pluginPackage `json:"package"`
	Credentials      Credentials   `json:"credentials"`
	ConnectRequested bool          `json:"connect_requested"`
}

type entry struct {
	record         record
	generation     uint64
	connection     *connection
	credentialHash [32]byte
	pendingCancel  context.CancelFunc
}

type gateway struct {
	server        *mcp.Server
	handler       http.Handler
	revision      uint64
	readOnly      bool
	platformTools map[string]platformTool
}

type Service struct {
	mu            sync.Mutex
	path          string
	dataRoot      string
	ephemeralData bool
	options       Options
	packages      map[string]pluginPackage
	entries       map[string]*entry
	gateways      map[string]*gateway
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewService(statePath string, options Options) (*Service, error) {
	if statePath != "" {
		if err := os.Chmod(statePath, 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("protect App state permissions: %w", err)
		}
	}
	packages, err := loadPackages()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{path: statePath, options: options, packages: packages, entries: map[string]*entry{}, gateways: map[string]*gateway{}, ctx: ctx, cancel: cancel}
	if statePath != "" {
		s.dataRoot, err = filepath.Abs(filepath.Join(filepath.Dir(statePath), "apps"))
	} else {
		s.dataRoot, err = os.MkdirTemp("", "csgclaw-apps-")
		s.ephemeralData = true
	}
	if err != nil {
		cancel()
		return nil, fmt.Errorf("prepare App data root: %w", err)
	}
	var stored map[string]record
	if _, err := localstore.ReadSection(statePath, installationsSection, &stored); err != nil {
		cancel()
		return nil, err
	}
	for id, value := range stored {
		value.Tools = []*mcp.Tool{}
		value.CredentialsSet = nil
		if value.Disconnected {
			value.Status = "disconnected"
		} else {
			value.Status = "needs_configuration"
		}
		s.entries[id] = &entry{record: value}
	}
	return s, nil
}

func (s *Service) persistLocked(id string, r *record) error {
	return localstore.UpdateObjectSection(s.path, installationsSection, func(values map[string]json.RawMessage) error {
		if r == nil {
			delete(values, id)
			return nil
		}
		copy := *r
		copy.Tools = nil
		copy.CredentialsSet = nil
		raw, err := json.Marshal(copy)
		if err != nil {
			return err
		}
		values[id] = raw
		return nil
	})
}

func copyJSON[T any](v T) T {
	var out T
	b, _ := json.Marshal(v)
	_ = json.Unmarshal(b, &out)
	return out
}

func view(e *entry) Installation {
	result := copyJSON(e.record.Installation)
	result.Tools = []*mcp.Tool{}
	if e.connection != nil {
		result.Tools = copyJSON(e.connection.tools)
	}
	result.CredentialsSet = map[string]bool{"token": e.record.Credentials.Token != "", "app_id": e.record.Credentials.AppID != "", "app_secret": e.record.Credentials.AppSecret != ""}
	for key := range e.record.Credentials.Headers {
		result.CredentialsSet["headers."+key] = true
	}
	for key := range e.record.Credentials.Env {
		result.CredentialsSet["env."+key] = true
	}
	if e.record.Config.CredentialSource == "feishu_channel" {
		result.CredentialsSet["channel_reference"] = true
	}
	if !result.Enabled {
		result.Status = "disabled"
	}
	return result
}

func (s *Service) findLocked(agentID, id string) (*entry, error) {
	e := s.entries[id]
	if e == nil || e.record.AgentID != agentID {
		return nil, ErrNotFound
	}
	return e, nil
}

func (s *Service) List(_ context.Context, agentID string) ([]Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []Installation{}
	for _, e := range s.entries {
		if e.record.AgentID == agentID {
			items = append(items, view(e))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (s *Service) Get(_ context.Context, agentID, id string) (Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		return Installation{}, err
	}
	return view(e), nil
}

func (s *Service) uniqueNameLocked(agentID, name, except string) error {
	if strings.TrimSpace(name) == "" || len(name) > 160 {
		return fmt.Errorf("%w: a name of at most 160 bytes is required", ErrInvalid)
	}
	for id, e := range s.entries {
		if id != except && e.record.AgentID == agentID && e.record.Name == name {
			return ErrConflict
		}
	}
	return nil
}

// Protect all supplied headers/environment values, not only known secret names.
func protect(c *Config, credentials *Credentials) {
	if len(c.Headers) > 0 {
		if credentials.Headers == nil {
			credentials.Headers = map[string]string{}
		}
		for k, v := range c.Headers {
			credentials.Headers[k] = v
		}
		c.Headers = nil
	}
	if len(c.Env) > 0 {
		if credentials.Env == nil {
			credentials.Env = map[string]string{}
		}
		for k, v := range c.Env {
			credentials.Env[k] = v
		}
		c.Env = nil
	}
	if c.CredentialSource == "feishu_channel" {
		credentials.AppID = ""
		credentials.AppSecret = ""
	}
}

func mergeCredentials(old, next Credentials) Credentials {
	out := copyJSON(old)
	if next.Token != "" {
		out.Token = next.Token
	}
	if next.AppID != "" {
		out.AppID = next.AppID
	}
	if next.AppSecret != "" {
		out.AppSecret = next.AppSecret
	}
	if next.Headers != nil {
		if out.Headers == nil {
			out.Headers = map[string]string{}
		}
		for k, v := range next.Headers {
			if v == "" {
				delete(out.Headers, k)
			} else {
				out.Headers[k] = v
			}
		}
	}
	if next.Env != nil {
		if out.Env == nil {
			out.Env = map[string]string{}
		}
		for k, v := range next.Env {
			if v == "" {
				delete(out.Env, k)
			} else {
				out.Env[k] = v
			}
		}
	}
	return out
}

func (s *Service) Create(ctx context.Context, agentID string, in CreateRequest) (Installation, error) {
	pkg, ok := s.packages[in.AppID]
	if !ok {
		return Installation{}, ErrNotFound
	}
	if strings.TrimSpace(agentID) == "" {
		return Installation{}, ErrInvalid
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Config = normalizeConfig(in.Config, in.AppID)
	protect(&in.Config, &in.Credentials)
	if in.Config.AuthMode == "oauth2" {
		return Installation{}, ErrUnsupportedOAuth
	}
	if in.Connect {
		if err := validateConfig(in.Config, in.AppID); err != nil {
			return Installation{}, err
		}
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return Installation{}, err
	}
	id := hex.EncodeToString(random)
	now := time.Now().UTC()
	r := record{Installation: Installation{InstallationID: id, AgentID: agentID, AppID: in.AppID, Name: in.Name, Enabled: true, Status: "needs_configuration", Config: in.Config, CreatedAt: now, UpdatedAt: now}, Package: pkg, Credentials: in.Credentials}
	s.mu.Lock()
	if err := s.uniqueNameLocked(agentID, in.Name, ""); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	if _, err := s.prepareInstallation(r); err != nil {
		_ = s.removeInstallationData(id)
		s.mu.Unlock()
		return Installation{}, err
	}
	if err := s.persistLocked(id, &r); err != nil {
		_ = s.removeInstallationData(id)
		s.mu.Unlock()
		return Installation{}, err
	}
	s.entries[id] = &entry{record: r}
	result := view(s.entries[id])
	s.mu.Unlock()
	if in.Connect {
		connected, err := s.Connect(ctx, agentID, id)
		if err != nil && connected.InstallationID != "" {
			return connected, nil
		}
		return connected, err
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, agentID, id string, in UpdateRequest) (Installation, error) {
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	next := copyJSON(e.record)
	if in.Name != nil {
		next.Name = strings.TrimSpace(*in.Name)
		if err := s.uniqueNameLocked(agentID, next.Name, id); err != nil {
			s.mu.Unlock()
			return Installation{}, err
		}
	}
	if in.Enabled != nil {
		next.Enabled = *in.Enabled
	}
	if in.Config != nil {
		next.Config = normalizeConfig(*in.Config, next.AppID)
	}
	if in.Credentials != nil {
		next.Credentials = mergeCredentials(next.Credentials, *in.Credentials)
	}
	protect(&next.Config, &next.Credentials)
	if next.Config.AuthMode == "oauth2" {
		s.mu.Unlock()
		return Installation{}, ErrUnsupportedOAuth
	}
	next.UpdatedAt = time.Now().UTC()
	if err := s.persistLocked(id, &next); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	changed := in.Config != nil || in.Credentials != nil || in.Enabled != nil
	var old *connection
	var revision uint64
	if changed {
		old = e.connection
		revision = s.detachLocked(e)
		e.generation++
	}
	renamed := e.record.Name != next.Name
	e.record = next
	if !changed && renamed && e.connection != nil {
		revision = s.attachLocked(e)
	}
	if changed {
		if next.Disconnected {
			e.record.Status = "disconnected"
		} else {
			e.record.Status = "needs_configuration"
		}
	}
	reconnect := changed && next.Enabled && !next.Disconnected && next.ConnectRequested
	result := view(e)
	s.mu.Unlock()
	old.close()
	s.notify(agentID, revision)
	if reconnect {
		return s.connect(ctx, agentID, id, false)
	}
	return result, nil
}

func (s *Service) Connect(ctx context.Context, agentID, id string) (Installation, error) {
	return s.connect(ctx, agentID, id, true)
}

func (s *Service) connect(ctx context.Context, agentID, id string, explicit bool) (Installation, error) {
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	if !explicit && (e.record.Disconnected || !e.record.Enabled || !e.record.ConnectRequested) {
		out := view(e)
		s.mu.Unlock()
		return out, nil
	}
	r := copyJSON(e.record)
	if err := validateConfig(r.Config, r.AppID); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	dirs, err := s.prepareInstallation(r)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	if explicit {
		r.Disconnected = false
		r.ConnectRequested = true
	}
	r.Status = "connecting"
	r.LastError = ""
	r.UpdatedAt = time.Now().UTC()
	if err := s.persistLocked(id, &r); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	old := e.connection
	revision := s.detachLocked(e)
	e.generation++
	generation := e.generation
	e.record = r
	operationCtx, cancelOperation := context.WithCancel(ctx)
	e.pendingCancel = cancelOperation
	s.mu.Unlock()
	old.close()
	s.notify(agentID, revision)
	defer cancelOperation()
	conn, _, connectErr := s.open(operationCtx, agentID, r.Config, r.Credentials, dirs, func() { go s.refreshTools(agentID, id, generation) })
	s.mu.Lock()
	e, err = s.findLocked(agentID, id)
	if err != nil || e.generation != generation || s.ctx.Err() != nil {
		s.mu.Unlock()
		conn.close()
		return Installation{}, fmt.Errorf("app connection was superseded")
	}
	e.pendingCancel = nil
	if connectErr != nil {
		e.record.Status = "error"
		e.record.LastError = connectErr.Error()
		e.record.UpdatedAt = time.Now().UTC()
		persistErr := s.persistLocked(id, &e.record)
		out := view(e)
		s.mu.Unlock()
		conn.close()
		if persistErr != nil {
			return out, persistErr
		}
		return out, connectErr
	}
	conn.generation = generation
	e.connection = conn
	e.credentialHash = conn.credentialHash
	e.record.Status = "connected"
	e.record.LastError = ""
	e.record.UpdatedAt = time.Now().UTC()
	if err := s.persistLocked(id, &e.record); err != nil {
		e.connection = nil
		s.mu.Unlock()
		conn.close()
		return Installation{}, err
	}
	if e.record.Enabled {
		revision = s.attachLocked(e)
	}
	out := view(e)
	s.mu.Unlock()
	s.notify(agentID, revision)
	go s.watch(agentID, id, conn)
	return out, nil
}

func (s *Service) Disconnect(_ context.Context, agentID, id string) (Installation, error) {
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	r := copyJSON(e.record)
	r.Credentials = Credentials{}
	r.Disconnected = true
	r.ConnectRequested = false
	r.Status = "disconnected"
	r.LastError = ""
	r.UpdatedAt = time.Now().UTC()
	if err := s.persistLocked(id, &r); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	old := e.connection
	revision := s.detachLocked(e)
	e.generation++
	e.record = r
	out := view(e)
	s.mu.Unlock()
	old.close()
	s.notify(agentID, revision)
	return out, nil
}

func (s *Service) Delete(_ context.Context, agentID, id string) error {
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.persistLocked(id, nil); err != nil {
		s.mu.Unlock()
		return err
	}
	old := e.connection
	revision := s.detachLocked(e)
	delete(s.entries, id)
	s.mu.Unlock()
	old.close()
	s.notify(agentID, revision)
	return s.removeInstallationData(id)
}

func (s *Service) DeleteAgent(ctx context.Context, agentID string) error {
	items, _ := s.List(ctx, agentID)
	for _, item := range items {
		if err := s.Delete(ctx, agentID, item.InstallationID); err != nil {
			return err
		}
	}
	s.mu.Lock()
	g := s.gateways[agentID]
	delete(s.gateways, agentID)
	s.mu.Unlock()
	if g != nil {
		for session := range g.server.Sessions() {
			_ = session.Close()
		}
	}
	return nil
}

func (s *Service) Probe(ctx context.Context, agentID string, in ProbeRequest) (ProbeResult, error) {
	if in.InstallationID != "" {
		s.mu.Lock()
		e, err := s.findLocked(agentID, in.InstallationID)
		if err != nil {
			s.mu.Unlock()
			return ProbeResult{}, err
		}
		in.AppID = e.record.AppID
		in.Credentials = mergeCredentials(e.record.Credentials, in.Credentials)
		s.mu.Unlock()
	}
	if _, ok := s.packages[in.AppID]; !ok {
		return ProbeResult{}, ErrNotFound
	}
	in.Config = normalizeConfig(in.Config, in.AppID)
	protect(&in.Config, &in.Credentials)
	if err := validateConfig(in.Config, in.AppID); err != nil {
		return ProbeResult{}, err
	}
	var dirs processDirs
	if in.Config.Transport == "stdio" {
		root, err := os.MkdirTemp("", "csgclaw-app-probe-")
		if err != nil {
			return ProbeResult{}, fmt.Errorf("prepare App connection test")
		}
		defer os.RemoveAll(root)
		dirs, err = prepareProcessDirs(root, s.packages[in.AppID])
		if err != nil {
			return ProbeResult{}, err
		}
	}
	conn, result, err := s.open(ctx, agentID, in.Config, in.Credentials, dirs, nil)
	if err != nil {
		return ProbeResult{}, err
	}
	defer conn.close()
	return *result, nil
}

func (s *Service) Restore(ctx context.Context, agentID string) error {
	items, _ := s.List(ctx, agentID)
	var errs []error
	for _, item := range items {
		if _, err := s.connect(ctx, agentID, item.InstallationID, false); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) RestoreAll(ctx context.Context) error {
	s.mu.Lock()
	agents := map[string]bool{}
	for _, e := range s.entries {
		agents[e.record.AgentID] = true
	}
	s.mu.Unlock()
	var errs []error
	for agentID := range agents {
		if err := s.Restore(ctx, agentID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) channelHash(ctx context.Context, agentID string) ([32]byte, error) {
	if s.options.ResolveFeishu == nil {
		return [32]byte{}, fmt.Errorf("Feishu channel credentials are unavailable")
	}
	v, err := s.options.ResolveFeishu(ctx, agentID)
	if err != nil || v.AppID == "" || v.AppSecret == "" {
		return [32]byte{}, fmt.Errorf("Feishu channel credentials are unavailable")
	}
	return sha256.Sum256([]byte(v.AppID + "\x00" + v.AppSecret)), nil
}

func (s *Service) RefreshCredentials(ctx context.Context, agentID string) error {
	s.mu.Lock()
	ids := []string{}
	for id, e := range s.entries {
		if e.record.AgentID == agentID && e.record.Config.CredentialSource == "feishu_channel" && e.record.Enabled && !e.record.Disconnected && e.record.ConnectRequested {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()
	if len(ids) == 0 {
		return nil
	}
	fingerprint, hashErr := s.channelHash(ctx, agentID)
	var errs []error
	for _, id := range ids {
		s.mu.Lock()
		e, err := s.findLocked(agentID, id)
		changed := err == nil && (hashErr != nil || e.connection == nil || e.credentialHash != fingerprint)
		s.mu.Unlock()
		if changed {
			if _, err := s.connect(ctx, agentID, id, false); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Service) Close() error {
	s.cancel()
	s.mu.Lock()
	connections := []*connection{}
	servers := []*mcp.Server{}
	for _, e := range s.entries {
		if e.pendingCancel != nil {
			e.pendingCancel()
			e.pendingCancel = nil
			e.generation++
		}
		if e.connection != nil {
			connections = append(connections, e.connection)
			e.connection = nil
			e.generation++
		}
	}
	for _, g := range s.gateways {
		servers = append(servers, g.server)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		conn.close()
	}
	for _, server := range servers {
		for session := range server.Sessions() {
			_ = session.Close()
		}
	}
	if s.ephemeralData {
		return os.RemoveAll(s.dataRoot)
	}
	return nil
}
