package enginetest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
)

// InterfaceFactory creates one isolated Engine implementation with seeded
// Agents and programmable Turn behavior.
type InterfaceFactory func(testing.TB, []agentengine.Agent, TurnBehavior) agentengine.Interface

// RunInterfaceContract runs the same public Agent Engine behavior checks
// against any Interface implementation.
func RunInterfaceContract(t *testing.T, factory InterfaceFactory) {
	t.Helper()
	t.Run("agent lifecycle and secret handling", func(t *testing.T) {
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateStopped, "codex")}, nil)
		agents := client.Agents()
		created, err := agents.Create(context.Background(), agentengine.AgentCreateRequest{Spec: agentengine.AgentSpec{
			Name: "created", Role: agentengine.AgentRoleWorker,
			Runtime: agentengine.RuntimeSpec{Adapter: "openclaw", Sandboxed: true, Image: "contract-openclaw:latest"},
			Model:   agentengine.ModelSpec{ProviderID: "provider-a", ModelID: "model-a"},
		}})
		if err != nil || created.ID == "" || created.Spec.Runtime.Credentials != nil {
			t.Fatalf("Create() = %+v, %v", created, err)
		}

		got, err := agents.Get(context.Background(), "agent-a", agentengine.AgentGetOptions{})
		if err != nil || got.Spec.Runtime.Credentials != nil || got.Status.State != agentengine.AgentStateStopped {
			t.Fatalf("Get() = %+v, %v", got, err)
		}
		listed, err := agents.List(context.Background(), agentengine.AgentListOptions{})
		if err != nil || len(listed) != 2 {
			t.Fatalf("List() = %+v, %v", listed, err)
		}
		for _, item := range listed {
			if item.Spec.Runtime.Credentials != nil {
				t.Fatalf("List() leaked credentials: %+v", item)
			}
		}

		updatedSpec := got.Spec
		updatedSpec.Name = "updated"
		updatedSpec.Description = "updated description"
		updatedSpec.Skills = []string{"skill-creator"}
		updatedSpec.Runtime.Credentials = map[string]string{"secrets/token.txt": "contract-secret"}
		updatedSpec.Runtime.InitShell = "test -f secrets/token.txt"
		updatedSpec.MCPServers = map[string]agentengine.MCPServerConfig{
			"local":  {"command": "local-server"},
			"remote": {"url": "https://mcp.example.test"},
		}
		updated, err := agents.Update(context.Background(), got.ID, agentengine.AgentUpdateRequest{Spec: updatedSpec, ResourceVersion: got.ResourceVersion})
		if err != nil || updated.Spec.Name != "updated" || updated.Spec.Description != "updated description" || len(updated.Spec.MCPServers) != 2 || len(updated.Spec.Skills) != 1 || updated.Spec.Skills[0] != "skill-creator" || updated.Spec.Runtime.Credentials != nil || updated.Spec.Runtime.InitShell != "test -f secrets/token.txt" {
			t.Fatalf("Update() = %+v, %v", updated, err)
		}
		replacement := updated.Spec
		replacement.Skills = []string{"skill-installer"}
		replacement.Runtime.Credentials = map[string]string{"secrets/token.txt": "contract-secret"}
		replaced, err := agents.Update(context.Background(), got.ID, agentengine.AgentUpdateRequest{Spec: replacement, ResourceVersion: updated.ResourceVersion})
		if err != nil || len(replaced.Spec.Skills) != 1 || replaced.Spec.Skills[0] != "skill-installer" {
			t.Fatalf("complete Skill replacement = %+v, %v", replaced, err)
		}

		replaced.Spec.DesiredState = agentengine.AgentDesiredStateRunning
		started, err := agents.Update(context.Background(), got.ID, agentengine.AgentUpdateRequest{Spec: replaced.Spec, FieldMask: []string{"desired_state"}, ResourceVersion: replaced.ResourceVersion})
		if err != nil || started.Status.State != agentengine.AgentStateRunning || !started.Status.Ready {
			t.Fatalf("Start() = %+v, %v", started, err)
		}
		started.Spec.DesiredState = agentengine.AgentDesiredStateStopped
		stopped, err := agents.Update(context.Background(), got.ID, agentengine.AgentUpdateRequest{Spec: started.Spec, FieldMask: []string{"desired_state"}, ResourceVersion: started.ResourceVersion})
		if err != nil || stopped.Status.State != agentengine.AgentStateStopped || stopped.Status.Ready {
			t.Fatalf("Stop() = %+v, %v", stopped, err)
		}
		recreated, err := agents.Recreate(context.Background(), got.ID, agentengine.AgentRecreateOptions{})
		if err != nil || recreated.ID != got.ID || recreated.Status.State == "" || recreated.Status.RuntimeID == "" || recreated.Spec.DesiredState != agentengine.AgentDesiredStateRunning || recreated.Spec.Runtime.Credentials != nil {
			t.Fatalf("Recreate() = %+v, %v", recreated, err)
		}
		if err := agents.Delete(context.Background(), got.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if _, err := agents.Get(context.Background(), got.ID, agentengine.AgentGetOptions{}); agentengine.ErrorCodeOf(err) != agentengine.ErrorAgentUnavailable {
			t.Fatalf("Get(deleted) error = %v", err)
		}
	})

	t.Run("agent administration extensions", func(t *testing.T) {
		seed := contractAgent("agent-admin", agentengine.AgentStateStopped, "codex")
		client := factory(t, []agentengine.Agent{seed}, nil)
		agents := client.Agents()
		created, err := agents.Create(context.Background(), agentengine.AgentCreateRequest{
			ID: "agent-explicit",
			Spec: agentengine.AgentSpec{
				Name:    "explicit",
				Role:    agentengine.AgentRoleWorker,
				Runtime: agentengine.RuntimeSpec{Adapter: "openclaw", Sandboxed: true, Image: "contract-openclaw:latest"},
				Model:   agentengine.ModelSpec{Selector: "provider-a.model-a", ProviderID: "provider-a", ModelID: "model-a"},
			},
		})
		if err != nil || created.ID != "agent-explicit" || created.Spec.Model.Selector != "provider-a.model-a" || created.Spec.Runtime.Credentials != nil || created.Spec.Model.APIKey != "" {
			t.Fatalf("Create(explicit ID) = %+v, %v", created, err)
		}
		unchanged, err := agents.Update(context.Background(), created.ID, agentengine.AgentUpdateRequest{Spec: created.Spec, ResourceVersion: created.ResourceVersion})
		if err != nil || !unchanged.UpdatedAt.Equal(created.UpdatedAt) {
			t.Fatalf("no-op Update() = %+v, %v, want UpdatedAt %v", unchanged, err, created.UpdatedAt)
		}
		patchedSpec := created.Spec
		patchedSpec.Description = "patched"
		patched, err := agents.Update(context.Background(), created.ID, agentengine.AgentUpdateRequest{Spec: patchedSpec, FieldMask: []string{"description"}, ResourceVersion: unchanged.ResourceVersion})
		if err != nil || patched.Spec.Description != "patched" || patched.Spec.Runtime.Credentials != nil || patched.Spec.Model.APIKey != "" {
			t.Fatalf("Patch() = %+v, %v", patched, err)
		}
		staleSpec := patched.Spec
		staleSpec.Description = "stale overwrite"
		if _, err := agents.Update(context.Background(), created.ID, agentengine.AgentUpdateRequest{
			Spec: staleSpec, FieldMask: []string{"description"}, ResourceVersion: created.ResourceVersion,
		}); agentengine.ErrorCodeOf(err) != agentengine.ErrorInvalidRequest {
			t.Fatalf("stale ResourceVersion Update error = %v", err)
		}
		stillPatched, err := agents.Get(context.Background(), created.ID, agentengine.AgentGetOptions{})
		if err != nil || stillPatched.Spec.Description != "patched" {
			t.Fatalf("Agent after stale Update = %+v, %v", stillPatched, err)
		}
		patched.Spec.Skills = []string{"skill-creator"}
		withSkills, err := agents.Update(context.Background(), created.ID, agentengine.AgentUpdateRequest{
			Spec: patched.Spec, FieldMask: []string{"skills"}, ResourceVersion: patched.ResourceVersion,
		})
		if err != nil || len(withSkills.Spec.Skills) != 1 || withSkills.Spec.Skills[0] != "skill-creator" || withSkills.Spec.Description != "patched" {
			t.Fatalf("Skill field-mask Update() = %+v, %v", withSkills, err)
		}
		withSkills.Spec.MCPServers = map[string]agentengine.MCPServerConfig{"local": {"command": "local-server"}}
		withMCP, err := agents.Update(context.Background(), created.ID, agentengine.AgentUpdateRequest{
			Spec: withSkills.Spec, FieldMask: []string{"mcp_servers"}, ResourceVersion: withSkills.ResourceVersion,
		})
		if err != nil || len(withMCP.Spec.MCPServers) != 1 || len(withMCP.Spec.Skills) != 1 {
			t.Fatalf("MCP field-mask Update() = %+v, %v", withMCP, err)
		}
		withMCP.Spec.MCPServers = map[string]agentengine.MCPServerConfig{}
		clearedMCP, err := agents.Update(context.Background(), created.ID, agentengine.AgentUpdateRequest{
			Spec: withMCP.Spec, FieldMask: []string{"mcp_servers"}, ResourceVersion: withMCP.ResourceVersion,
		})
		if err != nil || clearedMCP.Spec.MCPServers == nil || len(clearedMCP.Spec.MCPServers) != 0 || len(clearedMCP.Spec.Skills) != 1 {
			t.Fatalf("explicit MCP clear Update() = %+v, %v", clearedMCP, err)
		}
		clearedMCP.Spec.Model.ReasoningEffort = "high"
		withModel, err := agents.Update(context.Background(), created.ID, agentengine.AgentUpdateRequest{
			Spec: clearedMCP.Spec, FieldMask: []string{"model"}, ResourceVersion: clearedMCP.ResourceVersion,
		})
		if err != nil || withModel.Spec.Model.ReasoningEffort != "high" || len(withModel.Spec.Skills) != 1 {
			t.Fatalf("Model field-mask Update() = %+v, %v", withModel, err)
		}
		got, err := agents.Get(context.Background(), created.ID, agentengine.AgentGetOptions{Reload: true, ProbeRuntime: true})
		if err != nil || got.Spec.Description != "patched" {
			t.Fatalf("Inspect() = %+v, %v", got, err)
		}
	})

	t.Run("run events result and recording", func(t *testing.T) {
		var publishedFile agentengine.OutputFile
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			events := []agentengine.TurnEvent{
				{Kind: agentengine.TurnEventThoughtDelta, Thought: "thinking"},
				{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool-1", Kind: "exec_command", InputSummary: "pwd"}},
				{Kind: agentengine.TurnEventActivityUpdate, Activity: &agentengine.ActivityUpdate{ID: "plan-1", Kind: "plan_update", Status: "running"}},
				{Kind: agentengine.TurnEventOutputItem, Output: &agentengine.OutputItem{Kind: agentengine.OutputItemResourceLink, Payload: activity.ResourceLink{Name: "report", URI: "https://example.com/report"}}},
				{Kind: agentengine.TurnEventTextDelta, Text: "answer"},
			}
			for _, event := range events {
				if err := sink.Emit(ctx, event); err != nil {
					return contractFailure(agentengine.ErrorRuntimeFailed, err)
				}
			}
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "answer", Files: []agentengine.OutputFile{publishedFile}, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		publishedFile = contractCreateFile(t, client, "agent-a")
		var events []agentengine.TurnEvent
		result := client.Conversations("agent-a").Run(context.Background(), contractTurn("turn-1", "conversation-1"), agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
			events = append(events, event)
			return nil
		}))
		if result.Status != agentengine.TurnSucceeded || result.Output != "answer" || len(result.Files) != 1 || !result.Dispatched || result.Error != nil {
			t.Fatalf("Run() = %+v", result)
		}
		if len(events) != 5 {
			t.Fatalf("events = %+v", events)
		}
		for index, event := range events {
			if event.TurnID != "turn-1" || event.Sequence != uint64(index+1) {
				t.Fatalf("event %d envelope = %+v", index, event)
			}
		}
		if events[0].Thought != "thinking" || events[1].Tool == nil || events[2].Activity == nil || events[3].Output == nil || events[4].Text != "answer" {
			t.Fatalf("normalized events = %+v", events)
		}
		file := result.Files[0]
		download, err := client.Conversations("agent-a").Files().Get(context.Background(), file.ID)
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(download.Content)
		closeErr := download.Content.Close()
		if download.Metadata != file.OutputFileMetadata || readErr != nil || closeErr != nil || string(content) != "contract-output" {
			t.Fatalf("file = %+v content = %q, read=%v close=%v", download.Metadata, content, readErr, closeErr)
		}
	})

	t.Run("agent role changes are rejected", func(t *testing.T) {
		worker := contractAgent("agent-worker", agentengine.AgentStateStopped, "codex")
		manager := contractAgent("agent-manager", agentengine.AgentStateStopped, "codex")
		manager.Spec.Role = agentengine.AgentRoleManager
		client := factory(t, []agentengine.Agent{worker, manager}, nil)
		workerUpdate, err := client.Agents().Get(context.Background(), worker.ID, agentengine.AgentGetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		workerUpdate.Spec.Role = agentengine.AgentRoleManager
		if _, err := client.Agents().Update(context.Background(), worker.ID, agentengine.AgentUpdateRequest{Spec: workerUpdate.Spec, ResourceVersion: workerUpdate.ResourceVersion}); agentengine.ErrorCodeOf(err) != agentengine.ErrorInvalidRequest {
			t.Fatalf("worker-to-manager Update error = %v", err)
		}
		managerUpdate, err := client.Agents().Get(context.Background(), manager.ID, agentengine.AgentGetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		managerUpdate.Spec.Role = agentengine.AgentRoleWorker
		if _, err := client.Agents().Update(context.Background(), manager.ID, agentengine.AgentUpdateRequest{Spec: managerUpdate.Spec, ResourceVersion: managerUpdate.ResourceVersion}); agentengine.ErrorCodeOf(err) != agentengine.ErrorInvalidRequest {
			t.Fatalf("manager-to-worker Update error = %v", err)
		}
	})

	t.Run("file resources and file input", func(t *testing.T) {
		client := factory(t, []agentengine.Agent{
			contractAgent("agent-a", agentengine.AgentStateRunning, "codex"),
			contractAgent("agent-b", agentengine.AgentStateRunning, "codex"),
		}, nil)
		content := []byte("contract file input")
		created, err := client.Conversations("agent-a").Files().Create(context.Background(), agentengine.FileCreateRequest{
			Name: "input.txt", MIMEType: "text/plain", SizeBytes: int64(len(content)),
		}, bytes.NewReader(content))
		if err != nil || created.ID == "" || created.Name != "input.txt" || created.SizeBytes != int64(len(content)) || len(created.SHA256) != 64 {
			t.Fatalf("Create file = %+v, %v", created, err)
		}
		if _, err := client.Conversations("agent-b").Files().Get(context.Background(), created.ID); agentengine.ErrorCodeOf(err) != agentengine.ErrorFileNotFound {
			t.Fatalf("cross-Agent Get error = %v", err)
		}
		download, err := client.Conversations("agent-a").Files().Get(context.Background(), created.ID)
		if err != nil {
			t.Fatal(err)
		}
		got, readErr := io.ReadAll(download.Content)
		closeErr := download.Content.Close()
		if download.Metadata != created.OutputFileMetadata || readErr != nil || closeErr != nil || !bytes.Equal(got, content) {
			t.Fatalf("file = %+v content = %q, read=%v close=%v", download.Metadata, got, readErr, closeErr)
		}
		request := contractTurn("turn-file-input", "conversation-file-input")
		request.Input = []agentengine.InputPart{{Kind: agentengine.InputPartFile, File: &agentengine.InputFile{ID: created.ID}}}
		if result := client.Conversations("agent-a").Run(context.Background(), request, nil); result.Status != agentengine.TurnSucceeded {
			t.Fatalf("file input Run = %+v", result)
		}
		if err := client.Conversations("agent-a").Files().Delete(context.Background(), created.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Conversations("agent-a").Files().Get(context.Background(), created.ID); agentengine.ErrorCodeOf(err) != agentengine.ErrorFileNotFound {
			t.Fatalf("Get deleted error = %v", err)
		}
		owned, err := client.Conversations("agent-a").Files().Create(context.Background(), agentengine.FileCreateRequest{
			Name: "owned.txt", MIMEType: "text/plain", SizeBytes: int64(len(content)),
		}, bytes.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Agents().Delete(context.Background(), "agent-a"); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Conversations("agent-a").Files().Get(context.Background(), owned.ID); agentengine.ErrorCodeOf(err) != agentengine.ErrorFileNotFound {
			t.Fatalf("Get deleted-Agent file error = %v", err)
		}
	})

	t.Run("conversation admission and concurrency", func(t *testing.T) {
		started := make(chan agentengine.ConversationKey, 2)
		release := make(chan struct{})
		behavior := func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
			started <- request.ConversationKey
			<-release
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		firstDone := make(chan agentengine.TurnResult, 1)
		go func() {
			firstDone <- conversations.Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		}()
		<-started
		overlap := conversations.Run(context.Background(), contractTurn("turn-2", "conversation-1"), nil)
		if overlap.Error == nil || overlap.Error.Code != agentengine.ErrorConversationBusy || overlap.Dispatched {
			t.Fatalf("overlap = %+v", overlap)
		}
		secondDone := make(chan agentengine.TurnResult, 1)
		go func() {
			secondDone <- conversations.Run(context.Background(), contractTurn("turn-3", "conversation-2"), nil)
		}()
		<-started
		close(release)
		if (<-firstDone).Status != agentengine.TurnSucceeded || (<-secondDone).Status != agentengine.TurnSucceeded {
			t.Fatal("different conversations did not complete")
		}
	})

	t.Run("turn retries are idempotent", func(t *testing.T) {
		var executions int
		var publishedFile agentengine.OutputFile
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			executions++
			if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "answer"}); err != nil {
				return contractFailure(agentengine.ErrorRuntimeFailed, err)
			}
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "answer", Files: []agentengine.OutputFile{publishedFile}, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		publishedFile = contractCreateFile(t, client, "agent-a")
		request := contractTurn("turn-1", "conversation-1")
		var firstEvents []agentengine.TurnEvent
		first := client.Conversations("agent-a").Run(context.Background(), request, agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
			firstEvents = append(firstEvents, event)
			return nil
		}))
		var retriedEvents []agentengine.TurnEvent
		retried := client.Conversations("agent-a").Run(context.Background(), request, agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
			retriedEvents = append(retriedEvents, event)
			return nil
		}))
		if executions != 1 {
			t.Fatalf("Runtime executions = %d, want 1", executions)
		}
		if first.Status != agentengine.TurnSucceeded || retried.Status != first.Status || retried.Output != first.Output || len(first.Files) != 1 || len(retried.Files) != 1 || retried.Files[0].ID != first.Files[0].ID || retried.Dispatched != first.Dispatched || agentengine.ErrorCodeOf(retried.Error) != agentengine.ErrorCodeOf(first.Error) {
			t.Fatalf("first = %+v, retried = %+v", first, retried)
		}
		if len(retriedEvents) != len(firstEvents) {
			t.Fatalf("first events = %+v, retried events = %+v", firstEvents, retriedEvents)
		}
		for index := range firstEvents {
			if retriedEvents[index].Sequence != firstEvents[index].Sequence || retriedEvents[index].Kind != firstEvents[index].Kind {
				t.Fatalf("event %d first = %+v, retried = %+v", index, firstEvents[index], retriedEvents[index])
			}
		}
		conflict := request
		conflict.Input = []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: "different"}}
		if result := client.Conversations("agent-a").Run(context.Background(), conflict, nil); result.Error == nil || result.Error.Code != agentengine.ErrorInvalidRequest || executions != 1 {
			t.Fatalf("conflicting retry = %+v, executions = %d", result, executions)
		}
	})

	t.Run("in-flight turn retries coalesce", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		var executions int
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			executions++
			close(started)
			if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "answer"}); err != nil {
				return contractFailure(agentengine.ErrorRuntimeFailed, err)
			}
			<-release
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "answer", Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		request := contractTurn("turn-1", "conversation-1")
		firstDone := make(chan agentengine.TurnResult, 1)
		secondDone := make(chan agentengine.TurnResult, 1)
		go func() { firstDone <- client.Conversations("agent-a").Run(context.Background(), request, nil) }()
		<-started
		go func() { secondDone <- client.Conversations("agent-a").Run(context.Background(), request, nil) }()
		select {
		case result := <-secondDone:
			t.Fatalf("retry returned before the original Turn completed: %+v", result)
		case <-time.After(20 * time.Millisecond):
		}
		close(release)
		first := <-firstDone
		second := <-secondDone
		if executions != 1 || first.Status != agentengine.TurnSucceeded || second.Status != agentengine.TurnSucceeded || second.Output != first.Output {
			t.Fatalf("executions = %d, first = %+v, second = %+v", executions, first, second)
		}
	})

	t.Run("admission wait serializes turns", func(t *testing.T) {
		started := make(chan agentengine.TurnID, 2)
		releaseFirst := make(chan struct{})
		behavior := func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
			started <- request.ID
			if request.ID == "turn-1" {
				<-releaseFirst
			}
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		firstDone := make(chan agentengine.TurnResult, 1)
		go func() {
			firstDone <- conversations.Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		}()
		if id := <-started; id != "turn-1" {
			t.Fatalf("first started Turn = %q", id)
		}
		waitRequest := contractTurn("turn-2", "conversation-1")
		waitRequest.Admission = agentengine.AdmissionWait
		secondDone := make(chan agentengine.TurnResult, 1)
		go func() { secondDone <- conversations.Run(context.Background(), waitRequest, nil) }()
		select {
		case id := <-started:
			t.Fatalf("waiting Turn started early: %q", id)
		case <-time.After(20 * time.Millisecond):
		}
		close(releaseFirst)
		if result := <-firstDone; result.Status != agentengine.TurnSucceeded {
			t.Fatalf("first Run() = %+v", result)
		}
		if id := <-started; id != "turn-2" {
			t.Fatalf("second started Turn = %q", id)
		}
		if result := <-secondDone; result.Status != agentengine.TurnSucceeded {
			t.Fatalf("second Run() = %+v", result)
		}
	})

	t.Run("admission supersede cancels before replacement", func(t *testing.T) {
		firstStarted := make(chan struct{})
		cleanupStarted := make(chan struct{})
		releaseCleanup := make(chan struct{})
		secondStarted := make(chan struct{})
		behavior := func(ctx context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
			if request.ID == "turn-1" {
				close(firstStarted)
				<-ctx.Done()
				close(cleanupStarted)
				<-releaseCleanup
				return agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: ctx.Err().Error()}}
			}
			close(secondStarted)
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		firstDone := make(chan agentengine.TurnResult, 1)
		go func() {
			firstDone <- conversations.Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		}()
		<-firstStarted
		replacement := contractTurn("turn-2", "conversation-1")
		replacement.Admission = agentengine.AdmissionSupersede
		secondDone := make(chan agentengine.TurnResult, 1)
		go func() { secondDone <- conversations.Run(context.Background(), replacement, nil) }()
		<-cleanupStarted
		select {
		case <-secondStarted:
			t.Fatal("replacement started before superseded Turn cleanup")
		case <-time.After(20 * time.Millisecond):
		}
		close(releaseCleanup)
		if result := <-firstDone; result.Status != agentengine.TurnCanceled {
			t.Fatalf("superseded Run() = %+v", result)
		}
		<-secondStarted
		if result := <-secondDone; result.Status != agentengine.TurnSucceeded {
			t.Fatalf("replacement Run() = %+v", result)
		}
	})

	t.Run("exact cancellation waits for cleanup", func(t *testing.T) {
		started := make(chan struct{})
		cleanupStarted := make(chan struct{})
		releaseCleanup := make(chan struct{})
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
			close(started)
			<-ctx.Done()
			close(cleanupStarted)
			<-releaseCleanup
			return agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: ctx.Err().Error()}}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		runDone := make(chan agentengine.TurnResult, 1)
		go func() {
			runDone <- conversations.Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		}()
		<-started
		if err := conversations.Cancel(context.Background(), "conversation-1", "other-turn"); err != nil {
			t.Fatalf("Cancel(other) error = %v", err)
		}
		cancelDone := make(chan error, 1)
		go func() { cancelDone <- conversations.Cancel(context.Background(), "conversation-1", "turn-1") }()
		<-cleanupStarted
		select {
		case err := <-cancelDone:
			t.Fatalf("Cancel returned before cleanup: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
		close(releaseCleanup)
		if err := <-cancelDone; err != nil {
			t.Fatal(err)
		}
		result := <-runDone
		if result.Status != agentengine.TurnCanceled || result.Error == nil || result.Error.Code != agentengine.ErrorCanceled || !result.Dispatched {
			t.Fatalf("canceled result = %+v", result)
		}
		if err := conversations.Cancel(context.Background(), "conversation-1", "turn-1"); err != nil {
			t.Fatalf("idempotent Cancel error = %v", err)
		}
	})

	t.Run("agent mutations drain active turns", func(t *testing.T) {
		operations := map[string]func(context.Context, agentengine.AgentInterface, agentengine.Agent) error{
			"stop": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				item.Spec.DesiredState = agentengine.AgentDesiredStateStopped
				_, err := agents.Update(ctx, item.ID, agentengine.AgentUpdateRequest{Spec: item.Spec, FieldMask: []string{"desired_state"}, ResourceVersion: item.ResourceVersion})
				return err
			},
			"update": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				item.Spec.Description = "updated after drain"
				_, err := agents.Update(ctx, item.ID, agentengine.AgentUpdateRequest{Spec: item.Spec, ResourceVersion: item.ResourceVersion})
				return err
			},
			"patch": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				item.Spec.Description = "patched after drain"
				_, err := agents.Update(ctx, item.ID, agentengine.AgentUpdateRequest{Spec: item.Spec, FieldMask: []string{"description"}, ResourceVersion: item.ResourceVersion})
				return err
			},
			"recreate": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				_, err := agents.Recreate(ctx, item.ID, agentengine.AgentRecreateOptions{})
				return err
			},
			"upgrade": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				// The contract fixture intentionally has no default image, so the
				// post-drain upgrade may report that configuration error. This case
				// verifies that the operation still waits for the active Turn first.
				_, _ = agents.Recreate(ctx, item.ID, agentengine.AgentRecreateOptions{UpgradeImage: true})
				return nil
			},
			"delete": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				return agents.Delete(ctx, item.ID)
			},
		}
		for name, operation := range operations {
			operation := operation
			t.Run(name, func(t *testing.T) {
				started := make(chan struct{})
				release := make(chan struct{})
				behavior := func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
					close(started)
					<-release
					return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
				}
				seed := contractAgent("agent-a", agentengine.AgentStateRunning, "codex")
				client := factory(t, []agentengine.Agent{seed}, behavior)
				current, err := client.Agents().Get(context.Background(), seed.ID, agentengine.AgentGetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				runDone := make(chan agentengine.TurnResult, 1)
				go func() {
					runDone <- client.Conversations(seed.ID).Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
				}()
				<-started
				operationDone := make(chan error, 1)
				go func() { operationDone <- operation(context.Background(), client.Agents(), current) }()
				select {
				case err := <-operationDone:
					t.Fatalf("operation returned before active Turn drained: %v", err)
				case <-time.After(20 * time.Millisecond):
				}
				close(release)
				if result := <-runDone; result.Status != agentengine.TurnSucceeded {
					t.Fatalf("Run() = %+v", result)
				}
				if err := <-operationDone; err != nil {
					t.Fatal(err)
				}
			})
		}
	})

	t.Run("lifecycle drain timeout preserves the running agent", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		behavior := func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
			close(started)
			<-release
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		seed := contractAgent("agent-a", agentengine.AgentStateRunning, "codex")
		client := factory(t, []agentengine.Agent{seed}, behavior)
		runDone := make(chan agentengine.TurnResult, 1)
		go func() {
			runDone <- client.Conversations(seed.ID).Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		}()
		<-started
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		seed.Spec.DesiredState = agentengine.AgentDesiredStateStopped
		if _, err := client.Agents().Update(ctx, seed.ID, agentengine.AgentUpdateRequest{Spec: seed.Spec, FieldMask: []string{"desired_state"}, ResourceVersion: seed.ResourceVersion}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop() error = %v", err)
		}
		got, err := client.Agents().Get(context.Background(), seed.ID, agentengine.AgentGetOptions{})
		if err != nil || got.Status.State != agentengine.AgentStateRunning || got.Status.RuntimeID != seed.Status.RuntimeID {
			t.Fatalf("Agent changed after timeout: %+v, %v", got, err)
		}
		close(release)
		if result := <-runDone; result.Status != agentengine.TurnSucceeded {
			t.Fatalf("Run() = %+v", result)
		}
	})

	t.Run("reset removes strict continuation mapping", func(t *testing.T) {
		behavior := func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		if result := conversations.Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil); result.Status != agentengine.TurnSucceeded {
			t.Fatalf("first Run() = %+v", result)
		}
		if err := conversations.Reset(context.Background(), "conversation-1"); err != nil {
			t.Fatal(err)
		}
		strict := contractTurn("turn-2", "conversation-1")
		strict.Continuation = agentengine.ContinuationRequireExisting
		result := conversations.Run(context.Background(), strict, nil)
		if result.Error == nil || result.Error.Code != agentengine.ErrorConversationNotResumable || result.Dispatched {
			t.Fatalf("strict Run() = %+v", result)
		}
	})

	t.Run("reset atomically cancels an active turn", func(t *testing.T) {
		started := make(chan struct{})
		cleanupStarted := make(chan struct{})
		releaseCleanup := make(chan struct{})
		forceRelease := make(chan struct{})
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
			close(started)
			select {
			case <-ctx.Done():
				close(cleanupStarted)
				<-releaseCleanup
				return agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: ctx.Err().Error()}}
			case <-forceRelease:
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
			}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		runDone := make(chan agentengine.TurnResult, 1)
		go func() {
			runDone <- conversations.Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		}()
		<-started
		resetDone := make(chan error, 1)
		go func() { resetDone <- conversations.Reset(context.Background(), "conversation-1") }()
		select {
		case <-cleanupStarted:
		case err := <-resetDone:
			close(forceRelease)
			<-runDone
			t.Fatalf("Reset returned before canceling the active Turn: %v", err)
		case <-time.After(time.Second):
			close(forceRelease)
			<-runDone
			t.Fatal("Reset did not cancel the active Turn")
		}
		if result := conversations.Run(context.Background(), contractTurn("turn-blocked", "conversation-1"), nil); result.Error == nil || result.Error.Code != agentengine.ErrorConversationBusy || result.Dispatched {
			close(releaseCleanup)
			<-runDone
			<-resetDone
			t.Fatalf("Run during Reset = %+v", result)
		}
		select {
		case err := <-resetDone:
			close(releaseCleanup)
			<-runDone
			t.Fatalf("Reset returned before Turn cleanup: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
		close(releaseCleanup)
		if result := <-runDone; result.Status != agentengine.TurnCanceled {
			t.Fatalf("Run() = %+v", result)
		}
		if err := <-resetDone; err != nil {
			t.Fatal(err)
		}
		strict := contractTurn("turn-2", "conversation-1")
		strict.Continuation = agentengine.ContinuationRequireExisting
		if result := conversations.Run(context.Background(), strict, nil); result.Error == nil || result.Error.Code != agentengine.ErrorConversationNotResumable || result.Dispatched {
			t.Fatalf("strict Run() = %+v", result)
		}
	})

	t.Run("interaction resolution", func(t *testing.T) {
		resolved := make(chan struct{})
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventInteractionRequest, Interaction: &agentengine.InteractionRequest{ID: "question-1", Kind: agentengine.InteractionUserInput, Title: "Continue?"}}); err != nil {
				return contractFailure(agentengine.ErrorRuntimeFailed, err)
			}
			<-resolved
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		interactionID := make(chan string, 1)
		runDone := make(chan agentengine.TurnResult, 1)
		go func() {
			request := contractTurn("turn-1", "conversation-1")
			request.Interaction = agentengine.InteractionResolve
			runDone <- conversations.Run(context.Background(), request, agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
				if event.Interaction != nil {
					interactionID <- event.Interaction.ID
				}
				return nil
			}))
		}()
		id := <-interactionID
		if err := conversations.Resolve(context.Background(), agentengine.InteractionResolution{
			ConversationKey: "conversation-1", InteractionID: id, ResponderID: "tester",
			Answers: map[string]agentengine.InteractionAnswer{"choice": {Values: []string{"Yes"}}},
		}); err != nil {
			t.Fatal(err)
		}
		close(resolved)
		if result := <-runDone; result.Status != agentengine.TurnSucceeded || !result.Dispatched {
			t.Fatalf("Run() = %+v", result)
		}
	})

	t.Run("interaction policies", func(t *testing.T) {
		interaction := agentengine.TurnEvent{Kind: agentengine.TurnEventInteractionRequest, Interaction: &agentengine.InteractionRequest{ID: "question-1", Kind: agentengine.InteractionUserInput}}
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			if err := sink.Emit(ctx, interaction); err != nil {
				return contractFailure(agentengine.ErrorRuntimeFailed, err)
			}
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		reject := contractTurn("turn-reject", "conversation-reject")
		reject.Interaction = agentengine.InteractionReject
		result := client.Conversations("agent-a").Run(context.Background(), reject, nil)
		if result.Status != agentengine.TurnFailed || result.Error == nil || result.Error.Code != agentengine.ErrorInteractionUnsupported || !result.Dispatched {
			t.Fatalf("reject Run() = %+v", result)
		}

		var emitted []agentengine.TurnEvent
		skip := contractTurn("turn-skip", "conversation-skip")
		skip.Interaction = agentengine.InteractionSkipUserInput
		result = client.Conversations("agent-a").Run(context.Background(), skip, agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
			emitted = append(emitted, event)
			return nil
		}))
		if result.Status != agentengine.TurnSucceeded || result.Error != nil || !result.Dispatched {
			t.Fatalf("skip Run() = %+v", result)
		}
		for _, event := range emitted {
			if event.Interaction != nil {
				t.Fatalf("skip_user_input exposed interaction: %+v", emitted)
			}
		}
	})

	t.Run("sink failure terminates the turn", func(t *testing.T) {
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "partial"}); err != nil {
				return contractFailure(agentengine.ErrorRuntimeFailed, err)
			}
			<-ctx.Done()
			return agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: ctx.Err().Error()}}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		result := client.Conversations("agent-a").Run(context.Background(), contractTurn("turn-1", "conversation-1"), agentengine.EventSinkFunc(func(context.Context, agentengine.TurnEvent) error {
			return errors.New("sink closed")
		}))
		if result.Status != agentengine.TurnFailed || result.Error == nil || result.Error.Code != agentengine.ErrorRuntimeFailed || !result.Dispatched {
			t.Fatalf("Run() = %+v", result)
		}
	})

	t.Run("stable validation and availability errors", func(t *testing.T) {
		client := factory(t, []agentengine.Agent{
			contractAgent("agent-running", agentengine.AgentStateRunning, "codex"),
			contractAgent("agent-stopped", agentengine.AgentStateStopped, "codex"),
			contractAgent("agent-unsupported", agentengine.AgentStateRunning, "openclaw_sandbox"),
		}, nil)
		invalid := client.Conversations("agent-running").Run(context.Background(), agentengine.TurnRequest{}, nil)
		if invalid.Error == nil || invalid.Error.Code != agentengine.ErrorInvalidRequest || invalid.Dispatched {
			t.Fatalf("invalid Run() = %+v", invalid)
		}
		invalidFile := contractTurn("turn-invalid-file", "conversation-invalid-file")
		invalidFile.Input = []agentengine.InputPart{{Kind: agentengine.InputPartFile, File: &agentengine.InputFile{ID: "file"}}}
		if result := client.Conversations("agent-running").Run(context.Background(), invalidFile, nil); result.Error == nil || result.Error.Code != agentengine.ErrorFileNotFound || result.Dispatched {
			t.Fatalf("invalid file Run() = %+v", result)
		}
		invalidContinuation := contractTurn("turn-invalid-continuation", "conversation-invalid-continuation")
		invalidContinuation.Continuation = agentengine.ContinuationPolicy("future")
		if result := client.Conversations("agent-running").Run(context.Background(), invalidContinuation, nil); result.Error == nil || result.Error.Code != agentengine.ErrorInvalidRequest || result.Dispatched {
			t.Fatalf("invalid continuation Run() = %+v", result)
		}
		invalidAdmission := contractTurn("turn-invalid-admission", "conversation-invalid-admission")
		invalidAdmission.Admission = agentengine.AdmissionPolicy("future")
		if result := client.Conversations("agent-running").Run(context.Background(), invalidAdmission, nil); result.Error == nil || result.Error.Code != agentengine.ErrorInvalidRequest || result.Dispatched {
			t.Fatalf("invalid admission Run() = %+v", result)
		}
		invalidInteraction := contractTurn("turn-invalid-interaction", "conversation-invalid-interaction")
		invalidInteraction.Interaction = agentengine.InteractionPolicy("future")
		if result := client.Conversations("agent-running").Run(context.Background(), invalidInteraction, nil); result.Error == nil || result.Error.Code != agentengine.ErrorInvalidRequest || result.Dispatched {
			t.Fatalf("invalid interaction Run() = %+v", result)
		}
		canceledContext, cancel := context.WithCancel(context.Background())
		cancel()
		canceled := client.Conversations("agent-running").Run(canceledContext, contractTurn("turn-canceled", "conversation-canceled"), nil)
		if canceled.Status != agentengine.TurnCanceled || canceled.Error == nil || canceled.Error.Code != agentengine.ErrorCanceled || canceled.Dispatched {
			t.Fatalf("pre-canceled Run() = %+v", canceled)
		}
		unavailable := client.Conversations("agent-stopped").Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		if unavailable.Error == nil || unavailable.Error.Code != agentengine.ErrorAgentUnavailable || unavailable.Dispatched {
			t.Fatalf("unavailable Run() = %+v", unavailable)
		}
		unsupported := client.Conversations("agent-unsupported").Run(context.Background(), contractTurn("turn-2", "conversation-1"), nil)
		if unsupported.Error == nil || unsupported.Error.Code != agentengine.ErrorRuntimeAdapterUnavailable || unsupported.Dispatched {
			t.Fatalf("unsupported Run() = %+v", unsupported)
		}
		unsupportedAgent, err := client.Agents().Get(context.Background(), "agent-unsupported", agentengine.AgentGetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		unsupportedAgent.Spec.Runtime.Credentials = map[string]string{"secret.txt": "secret"}
		if _, err := client.Agents().Update(context.Background(), unsupportedAgent.ID, agentengine.AgentUpdateRequest{Spec: unsupportedAgent.Spec, ResourceVersion: unsupportedAgent.ResourceVersion}); agentengine.ErrorCodeOf(err) != agentengine.ErrorUnsupportedRuntimeProvision {
			t.Fatalf("unsupported provisioning error = %v", err)
		}
	})
}

func contractAgent(id string, state agentengine.AgentState, adapter string) agentengine.Agent {
	return agentengine.Agent{
		ID: id,
		Spec: agentengine.AgentSpec{
			Name: id, Role: agentengine.AgentRoleWorker,
			Runtime: agentengine.RuntimeSpec{Adapter: adapter, Credentials: map[string]string{"seed": "must-not-leak"}},
			Model:   agentengine.ModelSpec{ProviderID: "provider-a", ModelID: "model-a"},
		},
		Status: agentengine.AgentStatus{State: state, RuntimeID: "runtime-" + id, Ready: state == agentengine.AgentStateRunning},
	}
}

func contractTurn(id agentengine.TurnID, key agentengine.ConversationKey) agentengine.TurnRequest {
	return agentengine.TurnRequest{
		ID: id, ConversationKey: key,
		Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: "hello"}},
	}
}

func contractFailure(code agentengine.ErrorCode, err error) agentengine.TurnResult {
	return agentengine.TurnResult{Status: agentengine.TurnFailed, Dispatched: true, Error: &agentengine.TurnError{Code: code, Message: err.Error()}}
}

func contractCreateFile(t testing.TB, client agentengine.Interface, agentID string) agentengine.OutputFile {
	t.Helper()
	content := []byte("contract-output")
	file, err := client.Conversations(agentID).Files().Create(context.Background(), agentengine.FileCreateRequest{
		Name: "contract.txt", MIMEType: "text/plain", SizeBytes: int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	return file
}
