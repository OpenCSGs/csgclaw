package relay

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"csgclaw/internal/apiclient"
	"github.com/google/uuid"
)

// Manager owns the server's published agents. Credentials stay on the server.
// The private manifest stores desired publications; executor files store results.
type Manager struct {
	identityMu           sync.Mutex
	boundSite, boundUser string
	opMu                 sync.Mutex
	mu                   sync.Mutex
	ctx                  context.Context
	cancel               context.CancelFunc
	wg                   sync.WaitGroup
	root                 string
	local                *apiclient.Client
	manifest             relayManifest
	running              map[string]*managedExecutor
	failures             map[string]string
	autoRunning          map[string]bool
	community            Community
	credentials          CredentialsSource
}
type managedExecutor struct {
	cancel    context.CancelFunc
	done      chan struct{}
	connected bool
}

// Deleted IDs are permanent: local agent IDs cannot be reused after deletion.
var ErrLocalAgentDeleted = errors.New("local agent has been deleted; create a new agent")

type relayManifest struct {
	Deleted      map[string]bool            `json:"deleted,omitempty"`
	Targets      map[string]CommunityTarget `json:"targets,omitempty"`
	Removing     map[string]bool            `json:"removing,omitempty"`
	AutoPublish  map[string]PublishRequest  `json:"auto_publish,omitempty"`
	Pending      map[string]bool            `json:"pending,omitempty"`
	ClientID     string                     `json:"client_id"`
	Site         string                     `json:"site,omitempty"`
	User         string                     `json:"user,omitempty"`
	Instances    map[string]int64           `json:"instances,omitempty"`
	ConnectionID string                     `json:"connection_id"`
	Catalog      map[string]CatalogEntry    `json:"catalog,omitempty"`
	Endpoint     string                     `json:"endpoint"`
	Token        string                     `json:"token"`
	Agents       map[string]PublishRequest  `json:"agents"`
}
type PublicationStatus struct {
	Pending      bool   `json:"pending,omitempty"`
	Removing     bool   `json:"removing,omitempty"`
	CanUnpublish bool   `json:"can_unpublish,omitempty"`
	Publishing   bool   `json:"publishing,omitempty"`
	Configured   bool   `json:"configured"`
	Endpoint     string `json:"endpoint"`
	Published    bool   `json:"published"`
	Agent        *Agent `json:"agent,omitempty"`
	Error        string `json:"error,omitempty"`
}

type ManagerOption func(*Manager)

func WithCommunity(community Community, credentials CredentialsSource) ManagerOption {
	return func(m *Manager) { m.community = community; m.credentials = credentials }
}
func (m *Manager) client(endpoint, token string) (*Client, error) {
	var identity SiteCredentials
	if m.credentials != nil {
		cred, err := m.boundCredentials()
		if err != nil {
			return nil, err
		}
		identity = cred
		endpoint, token = ResolveRelayProxy(cred.Site), cred.Token
	}
	c, err := NewClient(endpoint, token, nil)
	if err == nil {
		if m.credentials != nil {
			c.clientID = m.manifest.ClientID
			c.credentials = func() (SiteCredentials, error) {
				cred, err := m.boundCredentials()
				if err != nil {
					return cred, err
				}
				if cred.Site != identity.Site || cred.User != identity.User {
					return SiteCredentials{}, fmt.Errorf("community connection belongs to another login")
				}
				return cred, nil
			}
		}
	}
	return c, err
}
func (m *Manager) connectionReference() (string, error) {
	if m.credentials == nil {
		return m.manifest.ConnectionID, nil
	}
	cred, err := m.boundCredentials()
	if err != nil {
		return "", err
	}
	return m.connectionReferenceFor(cred), nil
}
func (m *Manager) connectionReferenceFor(cred SiteCredentials) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(m.manifest.ConnectionID+"\x00"+cred.Site+"\x00"+cred.User)))
}
func (m *Manager) boundCredentials() (SiteCredentials, error) {
	cred, err := m.credentials()
	if err != nil {
		return cred, err
	}
	m.identityMu.Lock()
	defer m.identityMu.Unlock()
	if m.boundSite != "" && (cred.Site != m.boundSite || cred.User != m.boundUser) {
		return SiteCredentials{}, fmt.Errorf("sign in to the original publication account and site")
	}
	return cred, nil
}
func isNotFound(err error) bool { var e *HTTPError; return errors.As(err, &e) && e.Status == 404 }

func NewManager(ctx context.Context, root string, local *apiclient.Client, options ...ManagerOption) (*Manager, error) {
	ctx, cancel := context.WithCancel(ctx)
	m := &Manager{ctx: ctx, cancel: cancel, root: root, local: local, running: map[string]*managedExecutor{}, failures: map[string]string{}, autoRunning: map[string]bool{}}
	for _, option := range options {
		option(m)
	}
	data, err := os.ReadFile(filepath.Join(root, "publications.json"))
	if err == nil {
		err = json.Unmarshal(data, &m.manifest)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		cancel()
		return nil, fmt.Errorf("invalid relay publication manifest")
	}
	m.boundSite, m.boundUser = m.manifest.Site, m.manifest.User
	if h, ok := m.community.(HubCommunity); ok {
		h.Credentials = m.boundCredentials
		m.community = h
	}
	if m.manifest.Deleted == nil {
		m.manifest.Deleted = map[string]bool{}
	}
	if m.manifest.Targets == nil {
		m.manifest.Targets = map[string]CommunityTarget{}
	}
	if m.manifest.Removing == nil {
		m.manifest.Removing = map[string]bool{}
	}
	if m.manifest.AutoPublish == nil {
		m.manifest.AutoPublish = map[string]PublishRequest{}
	}
	if m.manifest.Agents == nil {
		m.manifest.Agents = map[string]PublishRequest{}
	}
	if endpoint := os.Getenv("CSGCLAW_RELAY_URL"); endpoint != "" && m.manifest.Endpoint == "" {
		m.manifest.Endpoint = endpoint
	}
	if m.manifest.Endpoint == "" {
		m.manifest.Endpoint = "http://127.0.0.1:28080"
	}
	if token := os.Getenv("CSGCLAW_RELAY_TOKEN"); token != "" && m.manifest.Token == "" {
		m.manifest.Token = token
	}
	upgradeManifest := m.manifest.ClientID == "" || m.manifest.ConnectionID == ""
	if m.manifest.ClientID == "" {
		m.manifest.ClientID, err = StableClientID(root)
		if err != nil {
			cancel()
			return nil, err
		}
	}
	if m.manifest.Pending == nil {
		m.manifest.Pending = map[string]bool{}
	}
	if m.manifest.Instances == nil {
		m.manifest.Instances = map[string]int64{}
	}
	if m.credentials != nil {
		m.manifest.Token = ""
		m.manifest.Endpoint = ""
	}
	if m.manifest.ConnectionID == "" {
		m.manifest.ConnectionID = uuid.NewString()
	}
	if m.manifest.Catalog == nil {
		m.manifest.Catalog = map[string]CatalogEntry{}
	}
	if m.credentials != nil && len(m.manifest.Agents) == 0 && len(m.manifest.Instances) == 0 && len(m.manifest.AutoPublish) == 0 && len(m.manifest.Removing) == 0 && m.manifest.Site != "" {
		m.boundSite, m.boundUser = "", ""
		m.manifest.Site, m.manifest.User = "", ""
		upgradeManifest = true
	}
	if upgradeManifest {
		if err := m.saveLocked(); err != nil {
			cancel()
			return nil, err
		}
	}
	m.mu.Lock()
	for id, req := range m.manifest.Agents {
		if m.manifest.Removing[req.SourceAgentID] {
			continue
		}
		m.startLocked(id, req)
	}
	for localID := range m.manifest.AutoPublish {
		m.startAutoPublicationLocked(localID)
	}
	m.mu.Unlock()
	return m, nil
}
func (m *Manager) Close() {
	m.opMu.Lock()
	m.cancel()
	m.opMu.Unlock()
	m.wg.Wait()
}
func (m *Manager) saveLocked() error {
	if err := os.MkdirAll(m.root, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(m.manifest)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(m.root, ".publications-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
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
	return os.Rename(f.Name(), filepath.Join(m.root, "publications.json"))
}

// Configure is allowed only before publishing, avoiding accidental migration of
// live agents between relay identities. Omitted credentials preserve saved ones.
func (m *Manager) Configure(endpoint, token string) error {
	if m.credentials != nil {
		if endpoint != "" || token != "" {
			return fmt.Errorf("relay uses the current OpenCSG login")
		}
		_, err := m.boundCredentials()
		return err
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.manifest
	if endpoint != "" {
		next.Endpoint = endpoint
	}
	if token != "" {
		next.Token = token
	}
	if next.Endpoint == m.manifest.Endpoint && next.Token == m.manifest.Token {
		return nil
	}
	if len(m.manifest.Agents) > 0 || len(m.manifest.Instances) > 0 {
		return fmt.Errorf("unpublish agents before changing relay credentials")
	}
	if _, err := NewClient(next.Endpoint, next.Token, nil); err != nil {
		return err
	}
	next.ConnectionID = uuid.NewString()
	old := m.manifest
	m.manifest = next
	if err := m.saveLocked(); err != nil {
		m.manifest = old
		return err
	}
	return nil
}
func (m *Manager) Publish(ctx context.Context, req PublishRequest, avatar ...string) (Publication, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishLocked(ctx, req, avatar...)
}

// publishLocked serializes remote publication with queue cancellation/unpublish.
func (m *Manager) publishLocked(ctx context.Context, req PublishRequest, avatar ...string) (Publication, error) {
	if m.ctx.Err() != nil {
		return Publication{}, m.ctx.Err()
	}
	if m.manifest.Deleted[req.SourceAgentID] {
		return Publication{}, ErrLocalAgentDeleted
	}
	c, err := m.client(m.manifest.Endpoint, m.manifest.Token)
	if err != nil {
		return Publication{}, err
	}
	var identity SiteCredentials
	if m.community != nil {
		identity, err = m.boundCredentials()
		if err != nil {
			return Publication{}, err
		}
	}
	if m.manifest.Removing[req.SourceAgentID] {
		return Publication{}, fmt.Errorf("finish unpublishing this agent before publishing again")
	}
	target := m.publicationTargetLocked(req.SourceAgentID)
	// Keep a platform worker private until its community row confirms visibility.
	if target != nil {
		req.Visibility = "private"
	}
	p, err := c.Publish(ctx, uuid.NewString(), req)
	if err != nil {
		return p, err
	}
	// A failed first registration must not pin an otherwise unused installation.
	// Once relay confirms the registration, keep identity through partial recovery.
	if m.community != nil {
		m.identityMu.Lock()
		m.boundSite, m.boundUser = identity.Site, identity.User
		m.identityMu.Unlock()
		m.manifest.Site, m.manifest.User = identity.Site, identity.User
	}
	if !safeID(p.Agent.ID) {
		return p, fmt.Errorf("invalid relay agent ID")
	}
	old, exists := m.manifest.Agents[p.Agent.ID]
	oldEntry, hadEntry := m.manifest.Catalog[p.Agent.ID]
	entry := CatalogEntry{RelayURL: m.manifest.Endpoint, ConnectionID: m.manifest.ConnectionID, Agent: p.Agent, Avatar: oldEntry.Avatar}
	if len(avatar) > 0 {
		entry.Avatar = avatar[0]
	}
	m.manifest.Catalog[p.Agent.ID] = entry
	m.manifest.Agents[p.Agent.ID] = req
	if m.community != nil {
		m.manifest.Pending[p.Agent.ID] = true
	}
	if err = m.saveLocked(); err != nil {
		if hadEntry {
			m.manifest.Catalog[p.Agent.ID] = oldEntry
		} else {
			delete(m.manifest.Catalog, p.Agent.ID)
		}
		if exists {
			m.manifest.Agents[p.Agent.ID] = old
		} else {
			delete(m.manifest.Agents, p.Agent.ID)
		}
		return p, err
	}
	if m.community != nil {
		updated, instance, syncErr := m.syncCommunity(ctx, c, p.Agent.ID, req, target)
		if syncErr != nil {
			return p, syncErr
		}
		req = updated
		m.manifest.Agents[p.Agent.ID] = req
		if target == nil {
			m.manifest.Instances[req.SourceAgentID] = instance
		}
		p.Agent.Visibility = req.Visibility
	}
	delete(m.manifest.Pending, p.Agent.ID)
	if err = m.saveLocked(); err != nil {
		if m.community != nil {
			m.manifest.Pending[p.Agent.ID] = true
		}
		return p, err
	}
	delete(m.failures, p.Agent.ID)
	if _, ok := m.running[p.Agent.ID]; !ok {
		m.startLocked(p.Agent.ID, req)
	}
	return p, nil
}
func (m *Manager) startLocked(id string, req PublishRequest) {
	ctx, cancel := context.WithCancel(m.ctx)
	run := &managedExecutor{cancel: cancel, done: make(chan struct{})}
	m.running[id] = run
	endpoint, token := m.manifest.Endpoint, m.manifest.Token
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer close(run.done)
		var c *Client
		var err error
		backoff := time.Second
		for ctx.Err() == nil {
			m.mu.Lock()
			current, desired := m.manifest.Agents[id]
			if !desired || m.manifest.Removing[current.SourceAgentID] || m.manifest.Deleted[current.SourceAgentID] {
				m.mu.Unlock()
				break
			}
			c, err = m.client(endpoint, token)
			m.mu.Unlock()
			if err == nil {
				break
			}
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
			backoff = min(backoff*2, 15*time.Second)
		}
		if ctx.Err() != nil {
			err = nil
		}
		if c != nil && err == nil && ctx.Err() == nil {
			register := func() {
				registerCtx, stop := context.WithTimeout(ctx, 10*time.Second)
				defer stop()
				// Serialize retries with manual publication and re-check cancellation
				// after acquiring the lock. Unpublish waits for this worker to exit.
				m.mu.Lock()
				defer m.mu.Unlock()
				current, desired := m.manifest.Agents[id]
				if !desired || ctx.Err() != nil || m.manifest.Removing[current.SourceAgentID] {
					return
				}
				target := m.publicationTargetLocked(current.SourceAgentID)
				if m.community != nil {
					m.manifest.Pending[id] = true
					if err := m.saveLocked(); err != nil {
						return
					}
				}
				registration := current
				if target != nil {
					registration.Visibility = "private"
				}
				p, registerErr := c.Publish(registerCtx, uuid.NewString(), registration)
				if registerErr != nil || p.Agent.ID != id || m.community == nil {
					return
				}
				updated, instance, syncErr := m.syncCommunity(registerCtx, c, id, registration, target)
				if syncErr != nil {
					return
				}
				m.manifest.Agents[id] = updated
				if target == nil {
					m.manifest.Instances[current.SourceAgentID] = instance
				}
				delete(m.manifest.Pending, id)
				if err := m.saveLocked(); err != nil {
					m.manifest.Pending[id] = true
				}
			}
			register()
			retryDone := make(chan struct{})
			go func() {
				defer close(retryDone)
				backoff := time.Second
				nextVisibilityCheck := time.Now().Add(15 * time.Second)
				for {
					timer := time.NewTimer(backoff)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
					m.mu.Lock()
					pending := m.manifest.Pending[id]
					m.mu.Unlock()
					if pending {
						register()
						backoff = min(backoff*2, 15*time.Second)
					} else {
						backoff = time.Second
						if time.Now().After(nextVisibilityCheck) {
							m.refreshPlatformVisibility(ctx, c, id)
							nextVisibilityCheck = time.Now().Add(15 * time.Second)
						}
					}
				}
			}()
			state := filepath.Join(m.root, fmt.Sprintf("%x.json", sha256.Sum256([]byte(endpoint+"\x00"+id))))
			e := &Executor{
				Client: c, AgentID: id, StatePath: state,
				Handler:     &LocalExecutor{Client: m.local, AgentID: req.SourceAgentID, Scope: endpoint + "\x00" + id},
				OnReconnect: register,
				OnConnection: func(connected bool) {
					m.mu.Lock()
					run.connected = connected
					m.mu.Unlock()
				},
			}
			err = e.Run(ctx)
			cancel()
			<-retryDone
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.failures[id] = "relay executor stopped; unpublish and publish again"
			if errors.Is(err, errExecutorOwnershipUnknown) {
				m.failures[id] = errExecutorOwnershipUnknown.Error()
			}
		}
		delete(m.running, id)
	}()
}
func (m *Manager) Status(ctx context.Context, localID string) PublicationStatus {
	m.mu.Lock()
	_, publishing := m.manifest.AutoPublish[localID]
	s := PublicationStatus{Publishing: publishing, Configured: m.manifest.Token != "", Endpoint: m.manifest.Endpoint}
	if m.credentials != nil {
		cred, err := m.boundCredentials()
		s.Configured = err == nil
		if err == nil {
			s.Endpoint = ResolveRelayProxy(cred.Site)
		}
	}
	var id string
	for remote, req := range m.manifest.Agents {
		if req.SourceAgentID == localID {
			id = remote
			break
		}
	}
	token := m.manifest.Token
	connected := m.running[id] != nil && m.running[id].connected
	s.Error = m.failures[id]
	if m.manifest.Removing[localID] {
		s.Error = "unpublish cleanup pending; retry unpublishing"
	}
	pending := m.manifest.Pending[id]
	s.Pending, s.Removing = pending, m.manifest.Removing[localID]
	_, hasInstance := m.manifest.Instances[localID]
	s.CanUnpublish = publishing || id != "" || hasInstance || s.Removing
	c, clientErr := m.client(s.Endpoint, token)
	m.mu.Unlock()
	if id == "" {
		return s
	}
	s.Published = !pending
	if pending {
		s.Error = "community subscription update pending; retry publishing"
	}
	err := clientErr
	var a Agent
	if err == nil {
		err = c.Request(ctx, http.MethodGet, "/v1/agents/"+url.PathEscape(id), "", nil, &a)
	}
	if err != nil {
		s.Error = "relay status unavailable"
		return s
	}
	if !connected {
		a.Status = "offline"
		a.StatusReason = "reconnecting"
	}
	s.Agent = &a
	return s
}
func (m *Manager) Unpublish(ctx context.Context, localID string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	// Persist cancellation before remote cleanup so a queued startup worker cannot
	// resurrect an agent after an explicit unpublish, including across restarts.
	if desired, exists := m.manifest.AutoPublish[localID]; exists {
		delete(m.manifest.AutoPublish, localID)
		if err := m.saveLocked(); err != nil {
			m.manifest.AutoPublish[localID] = desired
			return err
		}
	}
	// Cancelling a queue with no remote progress requires no working login/network.
	hasRemote := false
	for _, req := range m.manifest.Agents {
		hasRemote = hasRemote || req.SourceAgentID == localID
	}
	_, hasInstance := m.manifest.Instances[localID]
	if !hasRemote && !hasInstance && !m.manifest.Removing[localID] {
		return nil
	}
	if m.community != nil {
		if _, err := m.boundCredentials(); err != nil {
			return err
		}
		m.manifest.Removing[localID] = true
		if err := m.saveLocked(); err != nil {
			return err
		}
	}
	// Wait for every reconnect callback before touching the directory, otherwise
	// an external upsert could resurrect a just-deleted community row.
	for id, req := range m.manifest.Agents {
		if req.SourceAgentID != localID {
			continue
		}
		if run := m.running[id]; run != nil {
			run.cancel()
			m.mu.Unlock()
			<-run.done
			m.mu.Lock()
		}
	}
	if m.community != nil && m.publicationTargetLocked(localID) == nil {
		if instance, exists := m.manifest.Instances[localID]; exists {
			if err := m.community.Delete(ctx, instance); err != nil {
				return err
			}
		} else {
			// A remote commit may have succeeded even if its response was lost.
			for id, req := range m.manifest.Agents {
				if req.SourceAgentID == localID {
					if err := m.community.DeleteExternalRegistration(ctx, id); err != nil {
						return err
					}
				}
			}
		}
	}

	for id, req := range m.manifest.Agents {
		if req.SourceAgentID != localID {
			continue
		}
		c, err := m.client(m.manifest.Endpoint, m.manifest.Token)
		if err == nil {
			err = c.Request(ctx, http.MethodDelete, "/v1/agents/"+url.PathEscape(id), "", nil, nil)
		}
		var httpErr *HTTPError
		if err != nil && !(errors.As(err, &httpErr) && httpErr.Status == 404) {
			if m.community == nil {
				m.startLocked(id, req)
			}
			return err
		}
		entry, hadEntry := m.manifest.Catalog[id]
		delete(m.manifest.Catalog, id)
		delete(m.manifest.Agents, id)
		delete(m.manifest.Pending, id)
		if err = m.saveLocked(); err != nil {
			m.manifest.Agents[id] = req
			if hadEntry {
				m.manifest.Catalog[id] = entry
			}
			if m.community == nil {
				m.startLocked(id, req)
			}
			return err
		}
		delete(m.failures, id)
	}
	delete(m.manifest.Instances, localID)
	delete(m.manifest.Removing, localID)
	if m.credentials != nil && len(m.manifest.Instances) == 0 && len(m.manifest.Agents) == 0 {
		m.identityMu.Lock()
		m.boundSite, m.boundUser = "", ""
		m.identityMu.Unlock()
		m.manifest.Site, m.manifest.User = "", ""
	}
	if err := m.saveLocked(); err != nil {
		return err
	}
	return nil
}

// GuardAgentDeletion serializes local deletion with publication operations.
// The caller must release the guard after removing the local runtime record.
func (m *Manager) GuardAgentDeletion(localID string) (func(), error) {
	m.opMu.Lock()
	m.mu.Lock()
	defer m.mu.Unlock()
	_, queued := m.manifest.AutoPublish[localID]
	_, instance := m.manifest.Instances[localID]
	blocked := queued || instance || m.manifest.Removing[localID]
	for _, req := range m.manifest.Agents {
		blocked = blocked || req.SourceAgentID == localID
	}
	if blocked {
		m.opMu.Unlock()
		return nil, fmt.Errorf("unpublish or cancel the agent publication before deleting it")
	}
	return m.opMu.Unlock, nil
}

// CompleteAgentDeletion retires the ID and releases its platform target only
// after local deletion succeeds. The caller must hold GuardAgentDeletion's lock.
func (m *Manager) CompleteAgentDeletion(localID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	target, exists := m.manifest.Targets[localID]
	m.manifest.Deleted[localID] = true
	delete(m.manifest.Targets, localID)
	if err := m.saveLocked(); err != nil {
		// Local deletion already succeeded. Keep the in-memory tombstone even
		// on disk failure so requests waiting for the deletion lock stay blocked.
		if exists {
			m.manifest.Targets[localID] = target
		}
		return err
	}
	return nil
}

func (m *Manager) publicationTargetLocked(localID string) *CommunityTarget {
	if t, ok := m.manifest.Targets[localID]; ok {
		return &t
	}
	return nil
}
func (m *Manager) syncCommunity(ctx context.Context, c *Client, id string, req PublishRequest, target *CommunityTarget) (PublishRequest, int64, error) {
	instance, err := m.community.Register(ctx, id, req, target)
	if err != nil {
		return req, 0, err
	}
	updated, err := syncRelayVisibility(ctx, c, id, req, instance.Public)
	return updated, instance.ID, err
}

func (m *Manager) refreshPlatformVisibility(ctx context.Context, c *Client, id string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	req, exists := m.manifest.Agents[id]
	if ctx.Err() != nil || !exists || m.community == nil || m.manifest.Pending[id] || m.manifest.Removing[req.SourceAgentID] {
		return
	}
	target := m.publicationTargetLocked(req.SourceAgentID)
	if target == nil {
		return
	}
	instances, err := m.community.List(ctx)
	if err != nil {
		return
	}
	public := false
	for _, instance := range instances {
		if matchesPlatformRegistration(instance, id, target) {
			public = instance.Public
			break
		}
	}
	updated, err := syncRelayVisibility(ctx, c, id, req, public)
	if err == nil && updated.Visibility != req.Visibility {
		m.manifest.Agents[id] = updated
		if err := m.saveLocked(); err != nil {
			m.manifest.Pending[id] = true
		}
	}
}

func syncRelayVisibility(ctx context.Context, c *Client, id string, req PublishRequest, public bool) (PublishRequest, error) {
	visibility := "private"
	if public {
		visibility = "public"
	}
	if req.Visibility != visibility {
		req.Visibility = visibility
		p, err := c.Publish(ctx, uuid.NewString(), req)
		if err != nil {
			return req, err
		}
		if p.Agent.ID != id {
			return req, fmt.Errorf("relay identity changed while synchronizing visibility")
		}
	}
	return req, nil
}
