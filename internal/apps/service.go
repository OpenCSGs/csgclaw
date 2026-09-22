package apps

import (
	"context"
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
	record        record
	generation    uint64
	connection    *connection
	pendingCancel context.CancelFunc
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
	if err := s.loadResources(stored); err != nil {
		cancel()
		return nil, err
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
		raw, err := json.Marshal(persistedRecord(copy))
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
	if result.AppID == "feishu" {
		result.FeishuAppID = e.record.Credentials.AppID
	}
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
			items = append(items, s.viewLocked(e))
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
	return s.viewLocked(e), nil
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

func clearConnectorOwnedCredentials(config Config, credentials *Credentials) {
	if config.AuthMode != "connector" {
		return
	}
	credentials.Token = ""
	credentials.AppID = ""
	credentials.AppSecret = ""
	credentials.Env = nil
}

// Create is a convenience for creating a resource and optionally binding it.
// Public Agent management uses Bind so it cannot implicitly change shared resources.
func (s *Service) Create(ctx context.Context, agentID string, in CreateRequest) (Installation, error) {
	if agentID != "" {
		s.mu.Lock()
		err := s.uniqueNameLocked(agentID, strings.TrimSpace(in.Name), "")
		s.mu.Unlock()
		if err != nil {
			return Installation{}, err
		}
		connect := in.Connect
		in.Connect = false
		resource, err := s.Create(ctx, "", in)
		if err != nil {
			return Installation{}, err
		}
		return s.Bind(ctx, agentID, BindRequest{ResourceID: resource.InstallationID, Connect: connect})
	}
	pkg, ok := s.packages[in.AppID]
	if !ok {
		return Installation{}, ErrNotFound
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Config = normalizeConfig(in.Config, in.AppID, in.Credentials)
	clearConnectorOwnedCredentials(in.Config, &in.Credentials)
	protect(&in.Config, &in.Credentials)
	clearReferencedPlatformCredentials(in.Config, &in.Credentials)
	if in.Connect {
		if err := validateConfig(in.Config, in.AppID); err != nil {
			return Installation{}, err
		}
	}
	id, err := newInstallationID()
	if err != nil {
		return Installation{}, err
	}
	now := time.Now().UTC()
	r := record{Installation: Installation{InstallationID: id, AgentID: agentID, ResourceEnabled: true, AppID: in.AppID, Name: in.Name, Enabled: true, Status: "needs_configuration", Config: in.Config, CreatedAt: now, UpdatedAt: now}, Package: pkg, Credentials: in.Credentials}
	s.mu.Lock()
	if err := s.uniqueNameLocked(agentID, in.Name, ""); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	if err := s.persistLocked(id, &r); err != nil {
		_ = s.removeInstallationData(id)
		s.mu.Unlock()
		return Installation{}, err
	}
	s.entries[id] = &entry{record: r}
	result := s.viewLocked(s.entries[id])
	s.mu.Unlock()
	return result, nil
}

func (s *Service) Update(ctx context.Context, agentID, id string, in UpdateRequest) (Installation, error) {
	if agentID == "" {
		return s.updateResource(ctx, id, in)
	}
	if in.Name != nil || in.Config != nil || in.Credentials != nil {
		return Installation{}, fmt.Errorf("%w: change shared settings on the global App resource", ErrInvalid)
	}

	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	if in.Enabled == nil {
		out := s.viewLocked(e)
		s.mu.Unlock()
		return out, nil
	}
	next := copyJSON(e.record)
	next.Enabled = *in.Enabled
	next.LastError, next.LastErrorCode, next.LastErrorHTTPStatus = "", "", 0
	next.Status = "needs_configuration"
	if next.Disconnected {
		next.Status = "disconnected"
	}
	next.UpdatedAt = time.Now().UTC()
	if err := s.persistLocked(id, &next); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	old := e.connection
	revision := s.detachLocked(e)
	e.generation++
	e.record = next
	reconnect := next.active() && !next.Disconnected && next.ConnectRequested
	result := s.viewLocked(e)
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
	if agentID == "" {
		return Installation{}, ErrInvalid
	}
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	if !e.record.ResourceEnabled {
		out := s.viewLocked(e)
		s.mu.Unlock()
		return out, connectionError("app_resource_disabled", "This App is disabled in global resources.", 0, false)
	}
	if !explicit && (e.record.Disconnected || !e.record.active() || !e.record.ConnectRequested) {
		out := s.viewLocked(e)
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
	r.LastErrorCode = ""
	r.LastErrorHTTPStatus = 0
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
	conn, _, connectErr := s.open(operationCtx, agentID, r.AppID, r.Config, r.Credentials, dirs, func() { go s.refreshTools(agentID, id, generation) })
	s.mu.Lock()
	e, err = s.findLocked(agentID, id)
	if err != nil || e.generation != generation || s.ctx.Err() != nil {
		s.mu.Unlock()
		conn.close()
		return Installation{}, fmt.Errorf("app connection was superseded")
	}
	e.pendingCancel = nil
	if connectErr != nil {
		setConnectionError(&e.record.Installation, connectErr)
		e.record.UpdatedAt = time.Now().UTC()
		persistErr := s.persistLocked(id, &e.record)
		out := s.viewLocked(e)
		s.mu.Unlock()
		conn.close()
		if persistErr != nil {
			return out, persistErr
		}
		return out, connectErr
	}
	conn.generation = generation
	e.connection = conn
	e.record.Status = "connected"
	e.record.LastError = ""
	e.record.LastErrorCode = ""
	e.record.LastErrorHTTPStatus = 0
	e.record.UpdatedAt = time.Now().UTC()
	if err := s.persistLocked(id, &e.record); err != nil {
		e.connection = nil
		s.mu.Unlock()
		conn.close()
		return Installation{}, err
	}
	if e.record.active() {
		revision = s.attachLocked(e)
	}
	out := s.viewLocked(e)
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
	r.Disconnected = true
	r.ConnectRequested = false
	r.Status = "disconnected"
	r.LastError = ""
	r.LastErrorCode = ""
	r.LastErrorHTTPStatus = 0
	r.UpdatedAt = time.Now().UTC()
	if err := s.persistLocked(id, &r); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	old := e.connection
	revision := s.detachLocked(e)
	e.generation++
	e.record = r
	out := s.viewLocked(e)
	s.mu.Unlock()
	old.close()
	s.notify(agentID, revision)
	return out, nil
}

func (s *Service) Delete(ctx context.Context, agentID, id string) error {
	if agentID == "" {
		return s.deleteResource(ctx, id)
	}
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
		stored := e.record.Credentials
		if in.Config.AuthMode == "connector" || in.Config.AuthMode == "oauth2" {
			stored.Token = ""
		}
		in.Credentials = mergeCredentials(stored, in.Credentials)
		s.mu.Unlock()
	}
	if _, ok := s.packages[in.AppID]; !ok {
		return ProbeResult{}, ErrNotFound
	}
	in.Config = normalizeConfig(in.Config, in.AppID, in.Credentials)
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
	conn, result, err := s.open(ctx, agentID, in.AppID, in.Config, in.Credentials, dirs, nil)
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
		if e.record.AgentID != "" {
			agents[e.record.AgentID] = true
		}
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

// InvalidateConnector closes every live App session backed by connectorID.
// Connector credentials are copied into HTTP transports, so those transports
// must never survive a credential rotation or disconnect.
func (s *Service) InvalidateConnector(connectorID string) {
	connectorID = strings.TrimSpace(connectorID)
	if connectorID == "" {
		return
	}
	type detached struct {
		conn *connection
	}
	var connections []detached
	revisions := map[string]uint64{}
	s.mu.Lock()
	for id, e := range s.entries {
		if e.record.AgentID == "" || e.record.Config.AuthMode != "connector" || !strings.EqualFold(strings.TrimSpace(e.record.Config.ConnectorID), connectorID) {
			continue
		}
		conn := e.connection
		revision := s.detachLocked(e)
		e.generation++
		e.record.Status = "needs_configuration"
		e.record.LastError = ""
		e.record.LastErrorCode = ""
		e.record.LastErrorHTTPStatus = 0
		e.record.UpdatedAt = time.Now().UTC()
		_ = s.persistLocked(id, &e.record)
		if conn != nil {
			connections = append(connections, detached{conn: conn})
		}
		if revision != 0 {
			revisions[e.record.AgentID] = revision
		}
	}
	s.mu.Unlock()
	for _, item := range connections {
		item.conn.close()
	}
	for agentID, revision := range revisions {
		s.notify(agentID, revision)
	}
}

// RefreshConnector invalidates cached transports and reconnects every App that
// was configured to stay connected, across all Agents.
func (s *Service) RefreshConnector(ctx context.Context, connectorID string) error {
	s.InvalidateConnector(connectorID)
	type target struct{ agentID, id string }
	var targets []target
	s.mu.Lock()
	for id, e := range s.entries {
		if e.record.Config.AuthMode == "connector" && strings.EqualFold(strings.TrimSpace(e.record.Config.ConnectorID), strings.TrimSpace(connectorID)) &&
			e.record.Enabled && !e.record.Disconnected && e.record.ConnectRequested {
			targets = append(targets, target{agentID: e.record.AgentID, id: id})
		}
	}
	s.mu.Unlock()
	var errs []error
	for _, item := range targets {
		if _, err := s.connect(ctx, item.agentID, item.id, false); err != nil {
			errs = append(errs, err)
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
