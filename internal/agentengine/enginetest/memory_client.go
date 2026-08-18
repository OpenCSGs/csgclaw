// Package enginetest provides contract-compatible Agent Engine test clients.
package enginetest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"csgclaw/internal/agentengine"
)

// TurnBehavior programs one MemoryClient Run.
type TurnBehavior func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult

// TurnCall records one admitted Run and its ordered events and result.
type TurnCall struct {
	AgentID string
	Request agentengine.TurnRequest
	Events  []agentengine.TurnEvent
	Result  agentengine.TurnResult
}

// Resolution records one successful Resolve.
type Resolution struct {
	AgentID string
	Value   agentengine.InteractionResolution
}

// MemoryClient is a concurrency-safe, stateful implementation of the same
// Interface used by production adapters.
type MemoryClient struct {
	mu sync.Mutex

	agents        map[string]agentengine.Agent
	provisions    map[string]memoryProvision
	conversations map[memoryConversation]string
	active        map[memoryConversation]*memoryTurn
	behavior      TurnBehavior
	calls         []TurnCall
	resolutions   []Resolution
	nextAgentID   atomic.Uint64
}

type memoryProvision struct {
	credentials map[string]string
	initShell   string
}

type memoryConversation struct {
	agentID string
	key     agentengine.ConversationKey
}

type memoryTurn struct {
	id           agentengine.TurnID
	cancel       context.CancelFunc
	done         chan struct{}
	interactions map[string]agentengine.InteractionRequest
}

var errMemoryInteractionRejected = errors.New("memory interaction rejected")

// NewMemoryClient creates a client with defensively copied seeded Agents.
func NewMemoryClient(agents ...agentengine.Agent) *MemoryClient {
	client := &MemoryClient{
		agents:        make(map[string]agentengine.Agent),
		provisions:    make(map[string]memoryProvision),
		conversations: make(map[memoryConversation]string),
		active:        make(map[memoryConversation]*memoryTurn),
	}
	for _, item := range agents {
		client.provisions[item.ID] = memoryProvision{credentials: cloneStringMap(item.Spec.Runtime.Credentials), initShell: item.Spec.Runtime.InitShell}
		item = cloneAgent(item)
		item.Spec.Runtime.Credentials = nil
		client.agents[item.ID] = item
	}
	return client
}

// SetTurnBehavior replaces the programmable Run behavior.
func (c *MemoryClient) SetTurnBehavior(behavior TurnBehavior) {
	c.mu.Lock()
	c.behavior = behavior
	c.mu.Unlock()
}

// Calls returns defensive copies of all admitted Runs.
func (c *MemoryClient) Calls() []TurnCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]TurnCall, len(c.calls))
	for index := range c.calls {
		out[index] = cloneCall(c.calls[index])
	}
	return out
}

// Resolutions returns defensive copies of successful resolutions.
func (c *MemoryClient) Resolutions() []Resolution {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]Resolution(nil), c.resolutions...)
	for index := range out {
		out[index].Value = cloneResolution(out[index].Value)
	}
	return out
}

func (c *MemoryClient) Agents() agentengine.AgentInterface {
	return &memoryAgents{client: c}
}

func (c *MemoryClient) Conversations(agentID string) agentengine.ConversationInterface {
	return &memoryConversations{client: c, agentID: strings.TrimSpace(agentID)}
}

type memoryAgents struct {
	client *MemoryClient
}

func (a *memoryAgents) Create(_ context.Context, spec agentengine.AgentSpec) (agentengine.Agent, error) {
	spec = normalizeSpec(spec)
	if err := validateSpec(spec); err != nil {
		return agentengine.Agent{}, err
	}
	id := fmt.Sprintf("memory-agent-%d", a.client.nextAgentID.Add(1))
	now := time.Now().UTC()
	item := agentengine.Agent{ID: id, Spec: cloneSpec(spec), Status: agentengine.AgentStatus{State: agentengine.AgentStateStopped}, CreatedAt: now, UpdatedAt: now}
	item.Spec.Runtime.Credentials = nil
	a.client.mu.Lock()
	a.client.agents[id] = item
	a.client.provisions[id] = memoryProvision{credentials: cloneStringMap(spec.Runtime.Credentials), initShell: spec.Runtime.InitShell}
	a.client.mu.Unlock()
	return cloneAgent(item), nil
}

func (a *memoryAgents) Get(_ context.Context, agentID string) (agentengine.Agent, error) {
	a.client.mu.Lock()
	defer a.client.mu.Unlock()
	item, ok := a.client.agents[strings.TrimSpace(agentID)]
	if !ok {
		for _, candidate := range a.client.agents {
			if strings.EqualFold(candidate.Spec.Name, strings.TrimSpace(agentID)) {
				item, ok = candidate, true
				break
			}
		}
	}
	if !ok {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorAgentUnavailable, Message: fmt.Sprintf("agent %q not found", agentID)}
	}
	return cloneAgent(item), nil
}

func (a *memoryAgents) List(context.Context) ([]agentengine.Agent, error) {
	a.client.mu.Lock()
	defer a.client.mu.Unlock()
	out := make([]agentengine.Agent, 0, len(a.client.agents))
	for _, item := range a.client.agents {
		out = append(out, cloneAgent(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (a *memoryAgents) Update(ctx context.Context, agentID string, spec agentengine.AgentSpec) (agentengine.Agent, error) {
	spec = normalizeSpec(spec)
	if err := validateSpec(spec); err != nil {
		return agentengine.Agent{}, err
	}
	if err := a.waitForAgent(ctx, agentID); err != nil {
		return agentengine.Agent{}, err
	}
	a.client.mu.Lock()
	defer a.client.mu.Unlock()
	item, ok := a.client.agents[strings.TrimSpace(agentID)]
	if !ok {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorAgentUnavailable, Message: fmt.Sprintf("agent %q not found", agentID)}
	}
	if item.Spec.Role != spec.Role {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "agent role changes are not supported"}
	}
	item.Spec = cloneSpec(spec)
	item.Spec.Runtime.Credentials = nil
	item.UpdatedAt = time.Now().UTC()
	a.client.agents[item.ID] = item
	a.client.provisions[item.ID] = memoryProvision{credentials: cloneStringMap(spec.Runtime.Credentials), initShell: spec.Runtime.InitShell}
	return cloneAgent(item), nil
}

func (a *memoryAgents) Delete(ctx context.Context, agentID string) error {
	if err := a.waitForAgent(ctx, agentID); err != nil {
		return err
	}
	a.client.mu.Lock()
	delete(a.client.agents, strings.TrimSpace(agentID))
	delete(a.client.provisions, strings.TrimSpace(agentID))
	for key := range a.client.conversations {
		if key.agentID == strings.TrimSpace(agentID) {
			delete(a.client.conversations, key)
		}
	}
	a.client.mu.Unlock()
	return nil
}

func (a *memoryAgents) Start(_ context.Context, agentID string) (agentengine.Agent, error) {
	return a.setState(agentID, agentengine.AgentStateRunning, true)
}

func (a *memoryAgents) Stop(ctx context.Context, agentID string) (agentengine.Agent, error) {
	if err := a.waitForAgent(ctx, agentID); err != nil {
		return agentengine.Agent{}, err
	}
	return a.setState(agentID, agentengine.AgentStateStopped, false)
}

func (a *memoryAgents) Recreate(ctx context.Context, agentID string) (agentengine.Agent, error) {
	if err := a.waitForAgent(ctx, agentID); err != nil {
		return agentengine.Agent{}, err
	}
	return a.setState(agentID, agentengine.AgentStateRunning, true)
}

func (a *memoryAgents) setState(agentID string, state agentengine.AgentState, ready bool) (agentengine.Agent, error) {
	a.client.mu.Lock()
	defer a.client.mu.Unlock()
	item, ok := a.client.agents[strings.TrimSpace(agentID)]
	if !ok {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorAgentUnavailable, Message: fmt.Sprintf("agent %q not found", agentID)}
	}
	item.Status.State = state
	item.Status.Ready = ready
	if ready && item.Status.RuntimeID == "" {
		item.Status.RuntimeID = "memory-" + item.ID
	}
	item.UpdatedAt = time.Now().UTC()
	a.client.agents[item.ID] = item
	return cloneAgent(item), nil
}

func (a *memoryAgents) waitForAgent(ctx context.Context, agentID string) error {
	for {
		a.client.mu.Lock()
		var pending []*memoryTurn
		for key, turn := range a.client.active {
			if key.agentID == strings.TrimSpace(agentID) {
				pending = append(pending, turn)
			}
		}
		a.client.mu.Unlock()
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-pending[0].done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type memoryConversations struct {
	client  *MemoryClient
	agentID string
}

func (c *memoryConversations) Run(ctx context.Context, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	if ctx == nil {
		ctx = context.Background()
	}
	request.ID = agentengine.TurnID(strings.TrimSpace(string(request.ID)))
	request.ConversationKey = agentengine.ConversationKey(strings.TrimSpace(string(request.ConversationKey)))
	request.Continuation = agentengine.ContinuationPolicy(strings.TrimSpace(strings.ToLower(string(request.Continuation))))
	request.Interaction = agentengine.InteractionPolicy(strings.TrimSpace(strings.ToLower(string(request.Interaction))))
	if c.agentID == "" || request.ID == "" || request.ConversationKey == "" || len(request.Input) == 0 {
		return failed(agentengine.ErrorInvalidRequest, "agent ID, turn ID, conversation key, and input are required")
	}
	for _, part := range request.Input {
		if part.Kind == agentengine.InputPartText && (strings.TrimSpace(part.Text) == "" || part.File != nil) {
			return failed(agentengine.ErrorInvalidRequest, "text input must contain only non-empty text")
		}
		if part.Kind == agentengine.InputPartFile && (part.File == nil || strings.TrimSpace(part.Text) != "") {
			return failed(agentengine.ErrorInvalidRequest, "file input must include only a file")
		}
		if part.Kind == agentengine.InputPartFile && (strings.TrimSpace(part.File.ID) == "" || strings.TrimSpace(part.File.SourcePath) == "" || strings.TrimSpace(part.File.Name) == "" || strings.TrimSpace(part.File.MediaType) == "" || part.File.SizeBytes < 0 || len(strings.TrimSpace(part.File.SHA256)) != 64) {
			return failed(agentengine.ErrorInvalidRequest, "file ID, source path, name, media type, non-negative size, and SHA-256 are required")
		}
		if part.Kind != agentengine.InputPartText && part.Kind != agentengine.InputPartFile {
			return failed(agentengine.ErrorInvalidRequest, "unsupported input kind")
		}
	}
	if request.Continuation == "" {
		request.Continuation = agentengine.ContinuationCreateOrResume
	}
	if request.Interaction == "" {
		request.Interaction = agentengine.InteractionResolve
	}
	if request.Continuation != agentengine.ContinuationCreateOrResume && request.Continuation != agentengine.ContinuationRequireExisting {
		return failed(agentengine.ErrorInvalidRequest, fmt.Sprintf("unsupported continuation policy %q", request.Continuation))
	}
	if request.Interaction != agentengine.InteractionResolve && request.Interaction != agentengine.InteractionReject && request.Interaction != agentengine.InteractionSkipUserInput {
		return failed(agentengine.ErrorInvalidRequest, fmt.Sprintf("unsupported interaction policy %q", request.Interaction))
	}
	if err := ctx.Err(); err != nil {
		return agentengine.TurnResult{Status: agentengine.TurnCanceled, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: err.Error()}}
	}
	key := memoryConversation{agentID: c.agentID, key: request.ConversationKey}
	runCtx, cancel := context.WithCancel(ctx)
	turn := &memoryTurn{id: request.ID, cancel: cancel, done: make(chan struct{}), interactions: make(map[string]agentengine.InteractionRequest)}

	c.client.mu.Lock()
	agentItem, exists := c.client.agents[c.agentID]
	if !exists || agentItem.Status.State != agentengine.AgentStateRunning {
		c.client.mu.Unlock()
		cancel()
		return failed(agentengine.ErrorAgentUnavailable, "agent is unavailable")
	}
	if !strings.EqualFold(strings.TrimSpace(agentItem.Spec.Runtime.Adapter), "codex") {
		c.client.mu.Unlock()
		cancel()
		return failed(agentengine.ErrorRuntimeAdapterUnavailable, fmt.Sprintf("runtime adapter %q is unavailable", agentItem.Spec.Runtime.Adapter))
	}
	if c.client.active[key] != nil {
		c.client.mu.Unlock()
		cancel()
		return failed(agentengine.ErrorConversationBusy, "conversation already has an active turn")
	}
	if request.Continuation == agentengine.ContinuationRequireExisting && c.client.conversations[key] == "" {
		c.client.mu.Unlock()
		cancel()
		return failed(agentengine.ErrorConversationNotResumable, "conversation has no Runtime-native mapping")
	}
	if c.client.conversations[key] == "" {
		c.client.conversations[key] = "memory-conversation"
	}
	c.client.active[key] = turn
	behavior := c.client.behavior
	callIndex := len(c.client.calls)
	c.client.calls = append(c.client.calls, TurnCall{AgentID: c.agentID, Request: cloneRequest(request)})
	c.client.mu.Unlock()

	sequence := uint64(0)
	var policyMu sync.Mutex
	var policyResult *agentengine.TurnResult
	recordingSink := agentengine.EventSinkFunc(func(eventCtx context.Context, event agentengine.TurnEvent) error {
		if event.Interaction != nil {
			switch request.Interaction {
			case agentengine.InteractionReject:
				result := failed(agentengine.ErrorInteractionUnsupported, "Runtime interaction is not supported by this caller")
				result.Dispatched = true
				policyMu.Lock()
				policyResult = &result
				policyMu.Unlock()
				cancel()
				return errMemoryInteractionRejected
			case agentengine.InteractionSkipUserInput:
				c.client.mu.Lock()
				c.client.resolutions = append(c.client.resolutions, Resolution{AgentID: c.agentID, Value: agentengine.InteractionResolution{
					ConversationKey: request.ConversationKey,
					InteractionID:   event.Interaction.ID,
					ResponderID:     "agent-engine",
				}})
				c.client.mu.Unlock()
				return nil
			default:
				c.client.mu.Lock()
				turn.interactions[event.Interaction.ID] = *event.Interaction
				c.client.mu.Unlock()
			}
		}
		sequence++
		event.Sequence = sequence
		c.client.mu.Lock()
		c.client.calls[callIndex].Events = append(c.client.calls[callIndex].Events, event)
		c.client.mu.Unlock()
		if sink != nil {
			return sink.Emit(eventCtx, event)
		}
		return nil
	})

	result := agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
	if behavior != nil {
		result = behavior(runCtx, c.agentID, cloneRequest(request), recordingSink)
	}
	if runCtx.Err() != nil && result.Status == agentengine.TurnSucceeded {
		result = agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: result.Dispatched, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: runCtx.Err().Error()}}
	}
	policyMu.Lock()
	if policyResult != nil {
		result = *policyResult
	}
	policyMu.Unlock()
	cancel()
	c.client.mu.Lock()
	c.client.calls[callIndex].Result = result
	delete(c.client.active, key)
	c.client.mu.Unlock()
	close(turn.done)
	return result
}

func (c *memoryConversations) Cancel(ctx context.Context, key agentengine.ConversationKey, turnID agentengine.TurnID) error {
	c.client.mu.Lock()
	turn := c.client.active[memoryConversation{agentID: c.agentID, key: key}]
	if turn == nil || turn.id != turnID {
		c.client.mu.Unlock()
		return nil
	}
	turn.cancel()
	done := turn.done
	c.client.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *memoryConversations) Reset(_ context.Context, key agentengine.ConversationKey) error {
	identity := memoryConversation{agentID: c.agentID, key: key}
	c.client.mu.Lock()
	defer c.client.mu.Unlock()
	if c.client.active[identity] != nil {
		return &agentengine.TurnError{Code: agentengine.ErrorConversationBusy, Message: "conversation already has an active turn"}
	}
	delete(c.client.conversations, identity)
	return nil
}

func (c *memoryConversations) Resolve(_ context.Context, resolution agentengine.InteractionResolution) error {
	identity := memoryConversation{agentID: c.agentID, key: resolution.ConversationKey}
	c.client.mu.Lock()
	defer c.client.mu.Unlock()
	turn := c.client.active[identity]
	if turn == nil {
		return &agentengine.TurnError{Code: agentengine.ErrorInteractionNotFound, Message: "pending interaction was not found"}
	}
	if _, ok := turn.interactions[resolution.InteractionID]; !ok {
		return &agentengine.TurnError{Code: agentengine.ErrorInteractionNotFound, Message: "pending interaction was not found"}
	}
	delete(turn.interactions, resolution.InteractionID)
	c.client.resolutions = append(c.client.resolutions, Resolution{AgentID: c.agentID, Value: cloneResolution(resolution)})
	return nil
}

func validateSpec(spec agentengine.AgentSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Runtime.Adapter) == "" {
		return &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "agent name and Runtime adapter are required"}
	}
	if (len(spec.Runtime.Credentials) > 0 || strings.TrimSpace(spec.Runtime.InitShell) != "") && !strings.EqualFold(strings.TrimSpace(spec.Runtime.Adapter), "codex") {
		return &agentengine.TurnError{Code: agentengine.ErrorUnsupportedRuntimeProvision, Message: "Runtime credentials and initShell are supported only by Codex"}
	}
	if spec.Role != agentengine.AgentRoleWorker && spec.Role != agentengine.AgentRoleManager {
		return &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "agent role must be worker or manager"}
	}
	return nil
}

func normalizeSpec(spec agentengine.AgentSpec) agentengine.AgentSpec {
	if strings.TrimSpace(string(spec.Role)) == "" {
		spec.Role = agentengine.AgentRoleWorker
	}
	spec.Role = agentengine.AgentRole(strings.ToLower(strings.TrimSpace(string(spec.Role))))
	spec.Runtime.Adapter = strings.ToLower(strings.TrimSpace(spec.Runtime.Adapter))
	return spec
}

func failed(code agentengine.ErrorCode, message string) agentengine.TurnResult {
	return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: code, Message: message}}
}

func cloneAgent(input agentengine.Agent) agentengine.Agent {
	input.Spec = cloneSpec(input.Spec)
	input.Spec.Runtime.Credentials = nil
	return input
}

func cloneSpec(input agentengine.AgentSpec) agentengine.AgentSpec {
	input.Skills = append([]string(nil), input.Skills...)
	input.Runtime.Credentials = cloneStringMap(input.Runtime.Credentials)
	input.Runtime.Options = cloneAnyMap(input.Runtime.Options)
	input.Model.Options = cloneAnyMap(input.Model.Options)
	servers := input.MCPServers
	input.MCPServers = make(map[string]agentengine.MCPServerConfig, len(servers))
	for name, config := range servers {
		input.MCPServers[name] = agentengine.MCPServerConfig(cloneAnyMap(map[string]any(config)))
	}
	return input
}

func cloneRequest(input agentengine.TurnRequest) agentengine.TurnRequest {
	input.Input = append([]agentengine.InputPart(nil), input.Input...)
	for index := range input.Input {
		if input.Input[index].File != nil {
			fileCopy := *input.Input[index].File
			input.Input[index].File = &fileCopy
		}
	}
	return input
}

func cloneResolution(input agentengine.InteractionResolution) agentengine.InteractionResolution {
	input.Answers = make(map[string]agentengine.InteractionAnswer, len(input.Answers))
	for key, answer := range input.Answers {
		answer.Values = append([]string(nil), answer.Values...)
		input.Answers[key] = answer
	}
	return input
}

func cloneCall(input TurnCall) TurnCall {
	input.Request = cloneRequest(input.Request)
	events := make([]agentengine.TurnEvent, len(input.Events))
	for index, event := range input.Events {
		event.Tool = cloneTool(event.Tool)
		event.Activity = cloneActivity(event.Activity)
		event.Interaction = cloneInteraction(event.Interaction)
		event.Output = cloneOutput(event.Output)
		events[index] = event
	}
	input.Events = events
	return input
}

func cloneTool(input *agentengine.ToolActivity) *agentengine.ToolActivity {
	if input == nil {
		return nil
	}
	out := *input
	out.Payload = cloneAny(input.Payload)
	return &out
}

func cloneActivity(input *agentengine.ActivityUpdate) *agentengine.ActivityUpdate {
	if input == nil {
		return nil
	}
	out := *input
	out.Payload = cloneAny(input.Payload)
	return &out
}

func cloneInteraction(input *agentengine.InteractionRequest) *agentengine.InteractionRequest {
	if input == nil {
		return nil
	}
	out := *input
	out.Payload = cloneAny(input.Payload)
	return &out
}

func cloneOutput(input *agentengine.OutputItem) *agentengine.OutputItem {
	if input == nil {
		return nil
	}
	out := *input
	out.Payload = cloneAny(input.Payload)
	return &out
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(input any) any {
	switch value := input.(type) {
	case map[string]any:
		return cloneAnyMap(value)
	case []any:
		out := make([]any, len(value))
		for index := range value {
			out[index] = cloneAny(value[index])
		}
		return out
	case []string:
		return append([]string(nil), value...)
	default:
		return value
	}
}

var _ agentengine.Interface = (*MemoryClient)(nil)
