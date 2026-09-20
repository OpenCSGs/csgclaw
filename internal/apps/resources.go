package apps

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/localstore"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newInstallationID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (r record) active() bool { return r.Enabled && (r.ResourceID == "" || r.ResourceEnabled) }

// Bindings persist references and local intent only. Secret/configuration ownership
// belongs to the global row; hydrated binding values exist only in memory.
func persistedRecord(r record) any {
	if r.ResourceID == "" {
		return r
	}
	return struct {
		InstallationID   string    `json:"installation_id"`
		ResourceID       string    `json:"resource_id"`
		AgentID          string    `json:"agent_id"`
		Enabled          bool      `json:"enabled"`
		Disconnected     bool      `json:"disconnected"`
		ConnectRequested bool      `json:"connect_requested"`
		CreatedAt        time.Time `json:"created_at"`
		UpdatedAt        time.Time `json:"updated_at"`
	}{r.InstallationID, r.ResourceID, r.AgentID, r.Enabled, r.Disconnected, r.ConnectRequested, r.CreatedAt, r.UpdatedAt}
}
func hydrateBinding(r *record, resource record) {
	r.AppID, r.Name = resource.AppID, resource.Name
	r.Config, r.Credentials, r.Package = copyJSON(resource.Config), copyJSON(resource.Credentials), resource.Package
	r.ResourceEnabled = resource.Enabled
}
func (s *Service) persistAllLocked() error {
	return localstore.UpdateObjectSection(s.path, installationsSection, func(values map[string]json.RawMessage) error {
		clear(values)
		for id, e := range s.entries {
			r := copyJSON(e.record)
			r.Tools, r.CredentialsSet, r.Bindings = nil, nil, nil
			data, err := json.Marshal(persistedRecord(r))
			if err != nil {
				return err
			}
			values[id] = data
		}
		return nil
	})
}
func (s *Service) loadResources(stored map[string]record) error {
	ids := make([]string, 0, len(stored))
	for id := range stored {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	converted := false
	names := map[string]bool{}
	for _, r := range stored {
		if r.AgentID == "" {
			names[r.Name] = true
		}
	}
	// Convert existing installations once, preserving their IDs and therefore tool names.
	for _, id := range ids {
		r := stored[id]
		if r.AgentID != "" && r.ResourceID == "" {
			sum := sha256.Sum256([]byte("app-resource/" + id))
			resourceID := hex.EncodeToString(sum[:16])
			global := copyJSON(r)
			global.InstallationID, global.AgentID = resourceID, ""
			global.Enabled, global.Disconnected, global.ConnectRequested = true, false, false
			if names[global.Name] {
				global.Name = r.Name + " (" + r.AgentID + ")"
			}
			for n := 2; names[global.Name]; n++ {
				global.Name = fmt.Sprintf("%s (%s, %d)", r.Name, r.AgentID, n)
			}
			names[global.Name] = true

			stored[resourceID] = global
			r.ResourceID = resourceID
			stored[id] = r
			converted = true
		}
	}
	for id, r := range stored {
		if r.AgentID != "" {
			resource, ok := stored[r.ResourceID]
			if !ok || resource.AgentID != "" {
				return fmt.Errorf("App binding has no global resource")
			}
			hydrateBinding(&r, resource)
		}
		r.Tools, r.CredentialsSet, r.Bindings = nil, nil, nil
		r.LastError, r.LastErrorCode, r.LastErrorHTTPStatus = "", "", 0
		r.Status = "needs_configuration"
		if r.Disconnected {
			r.Status = "disconnected"
		}
		s.entries[id] = &entry{record: r}
	}
	if converted {
		return s.persistAllLocked()
	}
	return nil
}

func (s *Service) viewLocked(e *entry) Installation {
	item := view(e)
	if e.record.AgentID != "" {
		if !e.record.ResourceEnabled {
			item.Status = "disabled"
			item.Tools = []*mcp.Tool{}
		}
		return item
	}
	item.ResourceID = e.record.InstallationID
	item.ResourceEnabled = e.record.Enabled
	item.Status = "configured"
	if item.Config.CredentialSource == "feishu_channel" {
		item.Status = "agent_identity_required"
	}
	if !item.Enabled {
		item.Status = "disabled"
	}
	item.Bindings = []BindingSummary{}
	for _, bound := range s.entries {
		if bound.record.ResourceID != e.record.InstallationID {
			continue
		}
		b := view(bound)
		if !bound.record.active() {
			b.Status = "disabled"
		}
		item.Bindings = append(item.Bindings, BindingSummary{InstallationID: b.InstallationID, AgentID: b.AgentID, Enabled: b.Enabled, Status: b.Status, ToolCount: len(b.Tools)})
	}
	sort.Slice(item.Bindings, func(i, j int) bool { return item.Bindings[i].AgentID < item.Bindings[j].AgentID })
	return item
}

func (s *Service) Bind(ctx context.Context, agentID string, in BindRequest) (Installation, error) {
	if strings.TrimSpace(agentID) == "" {
		return Installation{}, ErrInvalid
	}
	s.mu.Lock()
	resource, err := s.findLocked("", in.ResourceID)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	for _, existing := range s.entries {
		if existing.record.AgentID == agentID && existing.record.ResourceID == in.ResourceID {
			s.mu.Unlock()
			return Installation{}, ErrConflict
		}
	}
	id, err := newInstallationID()
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	now := time.Now().UTC()
	r := record{Installation: Installation{InstallationID: id, ResourceID: in.ResourceID, AgentID: agentID, Enabled: true, Status: "needs_configuration", CreatedAt: now, UpdatedAt: now}, ConnectRequested: in.Connect}
	hydrateBinding(&r, resource.record)
	if _, err = s.prepareInstallation(r); err == nil {
		err = s.persistLocked(id, &r)
	}
	if err != nil {
		_ = s.removeInstallationData(id)
		s.mu.Unlock()
		return Installation{}, err
	}
	s.entries[id] = &entry{record: r}
	out := s.viewLocked(s.entries[id])
	s.mu.Unlock()
	if in.Connect {
		connected, err := s.Connect(ctx, agentID, id)
		if err != nil && connected.InstallationID != "" {
			return connected, nil
		}
		return connected, err
	}
	return out, nil
}

func (s *Service) updateResource(ctx context.Context, id string, in UpdateRequest) (Installation, error) {
	s.mu.Lock()
	resource, err := s.findLocked("", id)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	next := copyJSON(resource.record)
	if in.Name != nil {
		next.Name = strings.TrimSpace(*in.Name)
		if err = s.uniqueNameLocked("", next.Name, id); err != nil {
			s.mu.Unlock()
			return Installation{}, err
		}
	}
	if in.Enabled != nil {
		next.Enabled = *in.Enabled
	}
	if in.Credentials != nil {
		next.Credentials = mergeCredentials(next.Credentials, *in.Credentials)
	}
	if in.Config != nil {
		next.Config = normalizeConfig(*in.Config, next.AppID, next.Credentials)
	}
	protect(&next.Config, &next.Credentials)
	clearReferencedPlatformCredentials(next.Config, &next.Credentials)
	if next.Config.AuthMode == "oauth2" {
		s.mu.Unlock()
		return Installation{}, ErrUnsupportedOAuth
	}
	next.UpdatedAt = time.Now().UTC()
	if err = s.persistLocked(id, &next); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	resource.record = next
	type affected struct {
		agent, id string
		conn      *connection
		revision  uint64
		reconnect bool
	}
	changes := []affected{}
	for bindingID, e := range s.entries {
		if e.record.ResourceID != id {
			continue
		}
		old := e.connection
		revision := s.detachLocked(e)
		e.generation++
		hydrateBinding(&e.record, next)
		e.record.LastError, e.record.LastErrorCode, e.record.LastErrorHTTPStatus = "", "", 0
		e.record.Status = "needs_configuration"
		if e.record.Disconnected {
			e.record.Status = "disconnected"
		}
		changes = append(changes, affected{e.record.AgentID, bindingID, old, revision, e.record.active() && !e.record.Disconnected && e.record.ConnectRequested})
	}
	s.mu.Unlock()
	// Revoke every binding before network reconnects. An unavailable upstream must
	// not leave another Agent with stale tools or old credentials.
	for _, c := range changes {
		c.conn.close()
		s.notify(c.agent, c.revision)
	}
	var reconnects sync.WaitGroup
	for _, c := range changes {
		if c.reconnect {
			reconnects.Go(func() { _, _ = s.connect(ctx, c.agent, c.id, false) })
		}
	}
	reconnects.Wait()

	return s.Get(ctx, "", id)
}

func (s *Service) deleteResource(ctx context.Context, id string) error {
	s.mu.Lock()
	resource, err := s.findLocked("", id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	removed := map[string]*entry{id: resource}
	for bindingID, e := range s.entries {
		if e.record.ResourceID == id {
			removed[bindingID] = e
		}
	}
	for k := range removed {
		delete(s.entries, k)
	}
	if err = s.persistAllLocked(); err != nil {
		for k, e := range removed {
			s.entries[k] = e
		}
		s.mu.Unlock()
		return err
	}
	type affected struct {
		agent, id string
		conn      *connection
		revision  uint64
	}
	changes := []affected{}
	for bindingID, e := range removed {
		if e.record.AgentID == "" {
			continue
		}
		conn := e.connection
		rev := s.detachLocked(e)
		e.generation++
		changes = append(changes, affected{e.record.AgentID, bindingID, conn, rev})
	}
	s.mu.Unlock()
	var failures []error
	for _, c := range changes {
		c.conn.close()
		s.notify(c.agent, c.revision)
		if err := s.removeInstallationData(c.id); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)

}
