package dsh

import (
	"bytes"
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func (c *conversation) Compact(ctx context.Context, req contract.CompactRequest) (contract.CompactResult, error) {
	proc, release, err := c.runtime.processForTurn(ctx, c.runtimeID)
	if err != nil {
		return contract.CompactResult{}, err
	}
	defer release()
	proc.mu.Lock()
	key := ""
	for binding, session := range proc.meta.Sessions {
		if session == req.Usage.SessionID && contract.LogicalRuntimeSessionKey(binding) == string(req.ConversationKey) {
			key = binding
			break
		}
	}
	proc.mu.Unlock()
	if key == "" {
		return contract.CompactResult{Status: "unavailable", Reason: modelcap.CompactionBindingMissing}, nil
	}
	// Resume exactly the saved binding without applying current memory policy or
	// creating another native conversation.
	proc.mu.Lock()
	ready := proc.ready[req.Usage.SessionID]
	proc.mu.Unlock()
	if !ready {
		var resumed sessionResult
		if err := proc.client.call(ctx, "session/resume", map[string]any{"sessionId": req.Usage.SessionID, "cwd": proc.workspace, "mcpServers": proc.mcp}, &resumed, nil); err != nil {
			return contract.CompactResult{Status: "unavailable", Reason: modelcap.CompactionRestoreFailed}, nil
		}
		proc.mu.Lock()
		proc.ready[req.Usage.SessionID] = true
		proc.mu.Unlock()
	}
	var endpoint struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	data, err := os.ReadFile(filepath.Join(proc.root, "context-control.json"))
	if err != nil {
		return contract.CompactResult{Status: "unsupported"}, nil
	}
	if err = json.Unmarshal(data, &endpoint); err != nil {
		return contract.CompactResult{}, err
	}
	body, _ := json.Marshal(map[string]any{"session_id": req.Usage.SessionID})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(endpoint.Port)+"/compact", bytes.NewReader(body))
	if err != nil {
		return contract.CompactResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+endpoint.Token)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Minute}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return contract.CompactResult{Status: "uncertain"}, nil
	}
	defer response.Body.Close()
	var result contract.CompactResult
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		return contract.CompactResult{Status: "uncertain"}, nil
	}
	if result.Usage != nil {
		saved := req.Usage
		saved.UsedTokens = result.Usage.UsedTokens
		saved.Estimated = true
		saved.Compacting = false
		saved.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		result.Usage = &saved
		proc.mu.Lock()
		if proc.contextUsage == nil {
			proc.contextUsage = map[string]modelcap.ContextUsage{}
		}
		proc.contextUsage[req.Usage.SessionID] = *result.Usage
		proc.mu.Unlock()
	}
	return result, nil
}
