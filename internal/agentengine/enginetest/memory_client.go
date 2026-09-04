// Package enginetest provides contract-compatible Agent Engine test clients.
package enginetest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/interactionstate"
	"csgclaw/internal/agentengine/lifecycle"
	"csgclaw/internal/runtime/extensionstate"
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
	lifecycle    lifecycle.Coordinator
	interactions agentengine.InteractionCoordinator
	mu           sync.Mutex

	agents         map[string]agentengine.Agent
	provisions     map[string]memoryProvision
	conversations  map[memoryConversation]string
	active         map[memoryConversation]*memoryTurn
	controls       map[memoryConversation]*memoryControl
	completed      map[memoryTurnIdentity]memoryCompletedTurn
	completedOrder []memoryTurnIdentity
	behavior       TurnBehavior
	calls          []TurnCall
	resolutions    []Resolution
	files          *agentengine.FileStore
	extensions     map[string]map[string]agentengine.RuntimeExtension
	nextAgentID    atomic.Uint64
}

type memoryProvision struct {
	credentials map[string]string
	initShell   string
	modelAPIKey string
}

type memoryConversation struct {
	agentID string
	key     agentengine.ConversationKey
}

type memoryTurn struct {
	request      agentengine.TurnRequest
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
	interactions map[string]*memoryInteraction
	sequence     uint64
	events       []agentengine.TurnEvent
	result       agentengine.TurnResult
	callIndex    int
}

type memoryControl struct {
	done chan struct{}
}

type memoryTurnIdentity struct {
	memoryConversation
	id agentengine.TurnID
}

type memoryCompletedTurn struct {
	request agentengine.TurnRequest
	events  []agentengine.TurnEvent
	result  agentengine.TurnResult
}

type memoryInteraction struct {
	request   agentengine.InteractionRequest
	resolving bool
}

const maxMemoryCompletedTurns = 1024

var errMemoryInteractionRejected = errors.New("memory interaction rejected")

// NewMemoryClient creates a client with defensively copied seeded Agents.
func NewMemoryClient(agents ...agentengine.Agent) *MemoryClient {
	client := &MemoryClient{
		agents:        make(map[string]agentengine.Agent),
		provisions:    make(map[string]memoryProvision),
		conversations: make(map[memoryConversation]string),
		active:        make(map[memoryConversation]*memoryTurn),
		controls:      make(map[memoryConversation]*memoryControl),
		completed:     make(map[memoryTurnIdentity]memoryCompletedTurn),
		files:         agentengine.NewFileStore(),
		extensions:    make(map[string]map[string]agentengine.RuntimeExtension),
	}
	for _, item := range agents {
		client.provisions[item.ID] = memoryProvision{credentials: cloneStringMap(item.Spec.Runtime.Credentials), initShell: item.Spec.Runtime.InitShell, modelAPIKey: item.Spec.Model.APIKey}
		item.Status.Model = redactedModelView(item.Spec.Model, item.Spec.Model.APIKey)
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

func (c *MemoryClient) RuntimeExtensions(agentID string) agentengine.RuntimeExtensionInterface {
	return &memoryRuntimeExtensions{client: c, agentID: strings.TrimSpace(agentID)}
}

type memoryRuntimeExtensions struct {
	client  *MemoryClient
	agentID string
}

func (e *memoryRuntimeExtensions) Apply(ctx context.Context, request agentengine.RuntimeExtensionApplyRequest) (agentengine.RuntimeExtension, error) {
	if e == nil || e.client == nil || e.agentID == "" {
		return agentengine.RuntimeExtension{}, &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "agent ID is required"}
	}

	ctx, release, err := e.client.lifecycle.Mutation(ctx, e.agentID)
	if err != nil {
		return agentengine.RuntimeExtension{}, err
	}
	defer release()
	spec := request.Spec
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Kind = strings.TrimSpace(spec.Kind)
	spec.Source.Provider = strings.TrimSpace(spec.Source.Provider)
	spec.Source.Ref = strings.TrimSpace(spec.Source.Ref)
	if spec.FailurePolicy == "" {
		spec.FailurePolicy = agentengine.RuntimeExtensionOptional
	}
	if !extensionstate.ValidName(spec.Name) || spec.Kind == "" || spec.Source.Provider == "" || spec.Source.Ref == "" || (spec.FailurePolicy != agentengine.RuntimeExtensionOptional && spec.FailurePolicy != agentengine.RuntimeExtensionBlockRuntime) {
		return agentengine.RuntimeExtension{}, &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "runtime extension name, kind, source provider, and source ref are required"}
	}
	e.client.mu.Lock()
	defer e.client.mu.Unlock()
	if _, ok := e.client.agents[e.agentID]; !ok {
		return agentengine.RuntimeExtension{}, &agentengine.TurnError{Code: agentengine.ErrorAgentNotFound, Message: fmt.Sprintf("agent %q not found", e.agentID)}
	}
	items := e.client.extensions[e.agentID]
	if items == nil {
		items = make(map[string]agentengine.RuntimeExtension)
		e.client.extensions[e.agentID] = items
	}
	current, found := items[spec.Name]
	if request.ResourceVersion != "" && (!found || request.ResourceVersion != current.ResourceVersion) {
		return agentengine.RuntimeExtension{}, &agentengine.TurnError{Code: agentengine.ErrorRuntimeExtensionConflict, Message: "runtime extension resource version does not match"}
	}
	now := time.Now().UTC()
	if !found {
		current = agentengine.RuntimeExtension{AgentID: e.agentID, CreatedAt: now}
	}
	current.Spec = spec
	current.UpdatedAt = now
	current.ResourceVersion = now.Format(time.RFC3339Nano)
	current.Status.Generation++
	current.Status.ObservedGeneration = current.Status.Generation
	current.Status.SourceRevision = spec.Source.Ref
	current.Status.State = agentengine.RuntimeExtensionConfigured
	current.Status.RuntimeLoaded = e.client.agents[e.agentID].Status.Ready
	current.Status.CheckedAt = now
	current.Status.AppliedAt = now
	// The test Runtime has one deterministic source/driver pair. Unknown
	// capabilities must fail just like a production registry miss.
	var applyErr error
	if spec.Source.Provider != "contract" || spec.Kind != "contract" {
		current.Status.State = agentengine.RuntimeExtensionError
		current.Status.Reason = "extension_unsupported"
		if spec.Source.Provider != "contract" {
			current.Status.Reason = "source_unavailable"
		}
		current.Status.Message = "The test Runtime capability is not registered"
		current.Status.RuntimeLoaded = false
		current.Status.ObservedGeneration = 0
		applyErr = errors.New(current.Status.Message)
	}
	items[spec.Name] = current
	return current, applyErr
}

func (e *memoryRuntimeExtensions) Get(_ context.Context, name string) (agentengine.RuntimeExtension, error) {
	e.client.mu.Lock()
	defer e.client.mu.Unlock()
	item, ok := e.client.extensions[e.agentID][strings.TrimSpace(name)]
	if !ok {
		return agentengine.RuntimeExtension{}, &agentengine.TurnError{Code: agentengine.ErrorRuntimeExtensionNotFound, Message: fmt.Sprintf("runtime extension %q not found", name)}
	}
	return item, nil
}

func (e *memoryRuntimeExtensions) List(context.Context) ([]agentengine.RuntimeExtension, error) {
	e.client.mu.Lock()
	defer e.client.mu.Unlock()
	items := e.client.extensions[e.agentID]
	out := make([]agentengine.RuntimeExtension, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec.Name < out[j].Spec.Name })
	return out, nil
}

func (e *memoryRuntimeExtensions) Delete(ctx context.Context, name string) error {
	ctx, release, err := e.client.lifecycle.Mutation(ctx, e.agentID)
	if err != nil {
		return err
	}
	defer release()

	e.client.mu.Lock()
	defer e.client.mu.Unlock()
	name = strings.TrimSpace(name)
	if _, ok := e.client.extensions[e.agentID][name]; !ok {
		return &agentengine.TurnError{Code: agentengine.ErrorRuntimeExtensionNotFound, Message: fmt.Sprintf("runtime extension %q not found", name)}
	}
	delete(e.client.extensions[e.agentID], name)
	return nil
}

type memoryAgents struct {
	client *MemoryClient
}

func (c *MemoryClient) withExtensionReadinessLocked(item agentengine.Agent) agentengine.Agent {
	for _, extension := range c.extensions[item.ID] {
		if extension.Spec.FailurePolicy == agentengine.RuntimeExtensionBlockRuntime && (extension.Status.State != agentengine.RuntimeExtensionConfigured || !extension.Status.RuntimeLoaded || extension.Status.Generation != extension.Status.ObservedGeneration) {
			item.Status.Ready = false
			item.Status.Message = "Required Runtime extension is not loaded"
		}
	}
	return cloneAgent(item)
}
func (c *MemoryClient) syncExtensionLoadLocked(agentID string, running bool) {
	for name, item := range c.extensions[agentID] {
		item.Status.RuntimeLoaded = running && item.Status.State == agentengine.RuntimeExtensionConfigured
		c.extensions[agentID][name] = item
	}
}

func (a *memoryAgents) Create(_ context.Context, request agentengine.AgentCreateRequest) (agentengine.Agent, error) {
	spec := request.Spec
	spec = normalizeSpec(spec)
	if err := validateSpec(spec); err != nil {
		return agentengine.Agent{}, err
	}
	id := strings.TrimSpace(request.ID)
	if id == "" {
		id = fmt.Sprintf("memory-agent-%d", a.client.nextAgentID.Add(1))
	}
	now := time.Now().UTC()
	state := agentengine.AgentStateRunning
	ready := true
	if spec.DesiredState == agentengine.AgentDesiredStateStopped {
		state = agentengine.AgentStateStopped
		ready = false
	}
	item := agentengine.Agent{ID: id, ResourceVersion: now.Format(time.RFC3339Nano), Spec: cloneSpec(spec), Status: agentengine.AgentStatus{State: state, Ready: ready, Model: redactedModelView(spec.Model, spec.Model.APIKey)}, CreatedAt: now, UpdatedAt: now}
	if spec.Memory != nil {
		item.Status.Memory = &agentengine.MemoryStatus{Enabled: spec.Memory.Enabled, Ready: true, Name: "memory.md"}
	}
	item.Spec.Runtime.Credentials = nil
	item.Spec.Model.APIKey = ""
	a.client.mu.Lock()
	a.client.agents[id] = item
	a.client.provisions[id] = memoryProvision{credentials: cloneStringMap(spec.Runtime.Credentials), initShell: spec.Runtime.InitShell, modelAPIKey: spec.Model.APIKey}
	a.client.mu.Unlock()
	return cloneAgent(item), nil
}

func (a *memoryAgents) Get(_ context.Context, agentID string, _ agentengine.AgentGetOptions) (agentengine.Agent, error) {
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
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorAgentNotFound, Message: fmt.Sprintf("agent %q not found", agentID)}
	}
	return a.client.withExtensionReadinessLocked(item), nil
}

func (a *memoryAgents) List(context.Context, agentengine.AgentListOptions) ([]agentengine.Agent, error) {
	a.client.mu.Lock()
	defer a.client.mu.Unlock()
	out := make([]agentengine.Agent, 0, len(a.client.agents))
	for _, item := range a.client.agents {
		out = append(out, a.client.withExtensionReadinessLocked(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func mergeMemoryUpdate(current agentengine.AgentSpec, request agentengine.AgentUpdateRequest) agentengine.AgentSpec {
	next := cloneSpec(current)
	for _, field := range request.FieldMask {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "name":
			next.Name = request.Spec.Name
		case "description":
			next.Description = request.Spec.Description
		case "instructions":
			next.Instructions = request.Spec.Instructions
		case "role":
			next.Role = request.Spec.Role
		case "runtime":
			next.Runtime = request.Spec.Runtime
		case "runtime.image":
			next.Runtime.Image = request.Spec.Runtime.Image
		case "runtime.options":
			next.Runtime.Options = cloneAnyMap(request.Spec.Runtime.Options)
		case "runtime.credentials":
			next.Runtime.Credentials = cloneStringMap(request.Spec.Runtime.Credentials)
		case "runtime.init_shell":
			next.Runtime.InitShell = request.Spec.Runtime.InitShell
		case "model":
			next.Model = request.Spec.Model
		case "model.selector":
			next.Model.Selector = request.Spec.Model.Selector
		case "skills":
			next.Skills = append([]string(nil), request.Spec.Skills...)
		case "mcp_servers":
			next.MCPServers = cloneMCPServers(request.Spec.MCPServers)
		case "memory":
			if request.Spec.Memory == nil {
				next.Memory = nil
			} else {
				memory := *request.Spec.Memory
				next.Memory = &memory
			}
		case "desired_state":
			next.DesiredState = request.Spec.DesiredState
		}
	}
	return next
}

func (a *memoryAgents) Update(ctx context.Context, agentID string, request agentengine.AgentUpdateRequest) (agentengine.Agent, error) {
	invalidate := false
	defer func() {
		if invalidate {
			a.client.interactions.Interrupt(agentID, "", "", true)
		}
	}()
	spec := request.Spec
	ctx, release, err := a.client.lifecycle.Mutation(ctx, agentID)
	if err != nil {
		return agentengine.Agent{}, err
	}
	defer release()
	a.client.mu.Lock()
	defer a.client.mu.Unlock()
	item, ok := a.client.agents[strings.TrimSpace(agentID)]
	if !ok {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorAgentNotFound, Message: fmt.Sprintf("agent %q not found", agentID)}
	}
	if request.ResourceVersion != "" && request.ResourceVersion != item.ResourceVersion {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "agent resource version is stale"}
	}
	if len(request.FieldMask) > 0 {
		spec = mergeMemoryUpdate(item.Spec, request)
	}
	spec = normalizeSpec(spec)
	if err := validateSpec(spec); err != nil {
		return agentengine.Agent{}, err
	}
	if item.Spec.Role != spec.Role {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "agent role changes are not supported"}
	}
	provision := a.client.provisions[item.ID]
	runtimeCredentials := spec.Runtime.Credentials
	if runtimeCredentials == nil {
		runtimeCredentials = provision.credentials
	}
	modelAPIKey := spec.Model.APIKey
	if modelAPIKey == "" {
		modelAPIKey = provision.modelAPIKey
	}
	desired := cloneSpec(spec)
	desired.Runtime.Credentials = cloneStringMap(runtimeCredentials)
	desired.Model.APIKey = modelAPIKey
	current := cloneSpec(item.Spec)
	current.Runtime.Credentials = cloneStringMap(provision.credentials)
	current.Model.APIKey = provision.modelAPIKey
	if reflect.DeepEqual(current, desired) {
		return cloneAgent(item), nil
	}
	item.Spec = desired
	item.Spec.Runtime.Credentials = nil
	item.Spec.Model.APIKey = ""
	item.Status.Model = redactedModelView(desired.Model, modelAPIKey)
	if current.Instructions != desired.Instructions {
		item.Status.Instructions = &agentengine.InstructionsStatus{Effective: desired.Instructions}
	}
	if desired.Memory != nil {
		item.Status.Memory = &agentengine.MemoryStatus{Enabled: desired.Memory.Enabled, Ready: true, Name: "memory.md"}
	}
	item.UpdatedAt = time.Now().UTC()
	item.ResourceVersion = item.UpdatedAt.Format(time.RFC3339Nano)
	if desired.DesiredState == agentengine.AgentDesiredStateStopped {
		item.Status.State = agentengine.AgentStateStopped
		item.Status.Ready = false
	} else {
		item.Status.State = agentengine.AgentStateRunning
		item.Status.Ready = true
	}
	a.client.agents[item.ID] = item
	a.client.syncExtensionLoadLocked(item.ID, item.Status.State == agentengine.AgentStateRunning)
	a.client.provisions[item.ID] = memoryProvision{credentials: cloneStringMap(runtimeCredentials), initShell: spec.Runtime.InitShell, modelAPIKey: modelAPIKey}
	invalidate = item.Status.State == agentengine.AgentStateStopped
	return cloneAgent(item), nil
}

func (a *memoryAgents) Delete(ctx context.Context, agentID string) error {
	a.client.interactions.Interrupt(agentID, "", "", true)
	ctx, release, err := a.client.lifecycle.Mutation(ctx, agentID)
	if err != nil {
		return err
	}
	defer release()
	a.client.mu.Lock()
	delete(a.client.agents, strings.TrimSpace(agentID))
	delete(a.client.provisions, strings.TrimSpace(agentID))
	delete(a.client.extensions, strings.TrimSpace(agentID))
	for key := range a.client.conversations {
		if key.agentID == strings.TrimSpace(agentID) {
			delete(a.client.conversations, key)
		}
	}
	a.client.mu.Unlock()
	a.client.files.DeleteAgent(strings.TrimSpace(agentID))
	return nil
}

func (a *memoryAgents) Start(_ context.Context, agentID string) (agentengine.Agent, error) {
	return a.setState(agentID, agentengine.AgentStateRunning, true)
}

func (a *memoryAgents) Stop(ctx context.Context, agentID string) (agentengine.Agent, error) {
	ctx, release, err := a.client.lifecycle.Mutation(ctx, agentID)
	if err != nil {
		return agentengine.Agent{}, err
	}
	defer release()
	return a.setState(agentID, agentengine.AgentStateStopped, false)
}

func (a *memoryAgents) Recreate(ctx context.Context, agentID string, options agentengine.AgentRecreateOptions) (agentengine.Agent, error) {
	ctx, release, err := a.client.lifecycle.Mutation(ctx, agentID)
	if err != nil {
		return agentengine.Agent{}, err
	}
	defer release()
	if options.Update != nil {
		if options.UpgradeImage {
			return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "recreate update and image upgrade cannot be combined"}
		}
		if _, err := a.Update(ctx, agentID, *options.Update); err != nil {
			return agentengine.Agent{}, err
		}
	}
	item, err := a.setState(agentID, agentengine.AgentStateRunning, true)
	if err == nil {
		a.client.interactions.Interrupt(agentID, "", "", true)
	}
	return item, err
}

func (a *memoryAgents) setState(agentID string, state agentengine.AgentState, ready bool) (agentengine.Agent, error) {
	a.client.mu.Lock()
	defer a.client.mu.Unlock()
	item, ok := a.client.agents[strings.TrimSpace(agentID)]
	if !ok {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorAgentNotFound, Message: fmt.Sprintf("agent %q not found", agentID)}
	}
	item.Status.State = state
	item.Status.Ready = ready
	a.client.syncExtensionLoadLocked(item.ID, state == agentengine.AgentStateRunning)
	if state == agentengine.AgentStateStopped {
		item.Spec.DesiredState = agentengine.AgentDesiredStateStopped
	} else if state == agentengine.AgentStateRunning {
		item.Spec.DesiredState = agentengine.AgentDesiredStateRunning
	}
	if ready && item.Status.RuntimeID == "" {
		item.Status.RuntimeID = "memory-" + item.ID
	}
	item.UpdatedAt = time.Now().UTC()
	item.ResourceVersion = item.UpdatedAt.Format(time.RFC3339Nano)
	a.client.agents[item.ID] = item
	return cloneAgent(item), nil
}

type memoryConversations struct {
	client  *MemoryClient
	agentID string
}

func (c *memoryConversations) Files() agentengine.FileInterface {
	return c.client.files.Scope(c.agentID)
}

func (c *memoryConversations) Run(ctx context.Context, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	if ctx == nil {
		ctx = context.Background()
	}
	request.ID = agentengine.TurnID(strings.TrimSpace(string(request.ID)))
	request.ConversationKey = agentengine.ConversationKey(strings.TrimSpace(string(request.ConversationKey)))
	request.Admission = agentengine.AdmissionPolicy(strings.TrimSpace(strings.ToLower(string(request.Admission))))
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
		if part.Kind == agentengine.InputPartFile && strings.TrimSpace(part.File.ID) == "" {
			return failed(agentengine.ErrorInvalidRequest, "file ID is required")
		}
		if part.Kind == agentengine.InputPartFile {
			download, err := c.client.files.Scope(c.agentID).Get(ctx, part.File.ID)
			if err != nil {
				return failed(agentengine.ErrorFileNotFound, err.Error())
			}
			if err := download.Content.Close(); err != nil {
				return failed(agentengine.ErrorFileUnavailable, "file content is unavailable")
			}
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
	if request.Admission == "" {
		request.Admission = agentengine.AdmissionRejectIfBusy
	}
	if request.Admission != agentengine.AdmissionRejectIfBusy && request.Admission != agentengine.AdmissionWait && request.Admission != agentengine.AdmissionSupersede {
		return failed(agentengine.ErrorInvalidRequest, fmt.Sprintf("unsupported admission policy %q", request.Admission))
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
	turn, completed, existing, admissionResult := c.admit(ctx, key, request)
	if completed != nil {
		return replayMemoryEvents(ctx, sink, completed.events, completed.result)
	}
	if existing != nil {
		if err := waitMemory(ctx, existing.done); err != nil {
			return memoryContextResult(ctx, err)
		}
		return replayMemoryEvents(ctx, sink, existing.events, existing.result)
	}
	if admissionResult != nil {
		return *admissionResult
	}
	releaseExecution, leaseErr := c.client.lifecycle.Execution(turn.ctx, c.agentID)
	if leaseErr != nil {
		result := failed(agentengine.ErrorAgentUnavailable, "Agent lifecycle change is in progress")
		if turn.ctx.Err() != nil {
			result = memoryContextResult(turn.ctx, leaseErr)
		}
		turn.cancel()
		c.completeMemoryTurn(key, turn, result)
		return result
	}
	defer releaseExecution()
	c.client.interactions.Interrupt(c.agentID, request.ConversationKey, "", true)

	c.client.mu.Lock()
	agentItem, exists := c.client.agents[c.agentID]
	agentItem = c.client.withExtensionReadinessLocked(agentItem)
	if !exists || agentItem.Status.State != agentengine.AgentStateRunning || !agentItem.Status.Ready {
		c.client.mu.Unlock()
		result := failed(agentengine.ErrorAgentUnavailable, "agent is unavailable")
		turn.cancel()
		c.completeMemoryTurn(key, turn, result)
		return result
	}
	if !strings.EqualFold(strings.TrimSpace(agentItem.Spec.Runtime.Adapter), "codex") {
		c.client.mu.Unlock()
		result := failed(agentengine.ErrorRuntimeAdapterUnavailable, fmt.Sprintf("runtime adapter %q is unavailable", agentItem.Spec.Runtime.Adapter))
		turn.cancel()
		c.completeMemoryTurn(key, turn, result)
		return result
	}
	if request.Continuation == agentengine.ContinuationRequireExisting && c.client.conversations[key] == "" {
		c.client.mu.Unlock()
		result := failed(agentengine.ErrorConversationNotResumable, "conversation has no Runtime-native mapping")
		turn.cancel()
		c.completeMemoryTurn(key, turn, result)
		return result
	}
	if c.client.conversations[key] == "" {
		c.client.conversations[key] = "memory-conversation"
	}
	behavior := c.client.behavior
	turn.callIndex = len(c.client.calls)
	c.client.calls = append(c.client.calls, TurnCall{AgentID: c.agentID, Request: cloneRequest(request)})
	c.client.mu.Unlock()

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
				turn.cancel()
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
				// The common recorder below atomically publishes the pending interaction.
			}
		}
		return c.recordMemoryEvent(eventCtx, turn, sink, event)
	})

	result := agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
	if behavior != nil {
		result = behavior(turn.ctx, c.agentID, cloneRequest(request), recordingSink)
	}
	if turn.ctx.Err() != nil && result.Status == agentengine.TurnSucceeded {
		result = agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: result.Dispatched, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: turn.ctx.Err().Error()}}
	}
	policyMu.Lock()
	if policyResult != nil {
		result = *policyResult
	}
	policyMu.Unlock()
	if result.Status != agentengine.TurnSucceeded {
		result.Files = nil
	}
	if result.Status == agentengine.TurnSucceeded {
		c.client.interactions.CompleteTurn(c.agentID, request.ConversationKey, request.ID)
	} else {
		c.client.interactions.Interrupt(c.agentID, request.ConversationKey, request.ID, false)
	}
	if result.Status == agentengine.TurnSucceeded && request.Interaction == agentengine.InteractionResolve {
		var args *activity.RequestUserInputArgs
		for _, event := range turn.events {
			if event.Output != nil && event.Output.Kind == agentengine.OutputItemRequestUserInput {
				if value, ok := event.Output.Payload.(activity.RequestUserInputArgs); ok {
					args = &value
				}
			}
		}
		if args != nil {
			item, err := c.client.interactions.CreateDetached(c.agentID, request.ConversationKey, request.ID, *args, func(item agentengine.InteractionRequest) {
				event := agentengine.TurnEvent{Kind: agentengine.TurnEventActivityUpdate, Activity: &agentengine.ActivityUpdate{ID: item.ID, Kind: string(activity.RuntimeEventUserInputResolved), Payload: item.Payload}}
				if snapshot, ok := item.Payload.(activity.UserInputSnapshot); ok {
					event.Activity.Status = string(snapshot.Status)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				_ = c.recordMemoryEvent(ctx, turn, sink, event)
			})
			if err != nil {
				result = failed(agentengine.ErrorRuntimeFailed, err.Error())
			} else {
				result.Interactions = []agentengine.InteractionRequest{item}
			}
		}
	}
	turn.cancel()
	c.completeMemoryTurn(key, turn, result)
	return result
}

func (c *memoryConversations) Cancel(ctx context.Context, key agentengine.ConversationKey, turnID agentengine.TurnID) error {
	if ctx == nil {
		ctx = context.Background()
	}
	key = agentengine.ConversationKey(strings.TrimSpace(string(key)))
	turnID = agentengine.TurnID(strings.TrimSpace(string(turnID)))
	c.client.interactions.Interrupt(c.agentID, key, turnID, true)
	c.client.mu.Lock()
	turn := c.client.active[memoryConversation{agentID: c.agentID, key: key}]
	if turn == nil || turn.request.ID != turnID {
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

func (c *memoryConversations) Reset(ctx context.Context, key agentengine.ConversationKey) error {
	if ctx == nil {
		ctx = context.Background()
	}
	key = agentengine.ConversationKey(strings.TrimSpace(string(key)))
	if c.agentID == "" || key == "" {
		return &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "agent ID and conversation key are required"}
	}
	identity := memoryConversation{agentID: c.agentID, key: key}
	control, active, ok := c.beginMemoryControl(identity)
	if !ok {
		return &agentengine.TurnError{Code: agentengine.ErrorConversationBusy, Message: "conversation already has a control operation"}
	}
	defer c.endMemoryControl(identity, control)
	c.client.interactions.Interrupt(c.agentID, key, "", true)
	if active != nil {
		if err := waitMemory(ctx, active.done); err != nil {
			return err
		}
	}
	c.client.mu.Lock()
	delete(c.client.conversations, identity)
	c.client.mu.Unlock()
	return nil
}

func (c *memoryConversations) GetInteraction(ctx context.Context, key agentengine.ConversationKey, id string) (agentengine.InteractionRequest, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return agentengine.InteractionRequest{}, err
		}
	}
	return c.client.interactions.Get(c.agentID, key, id)
}

func (c *memoryConversations) Resolve(ctx context.Context, resolution agentengine.InteractionResolution) error {
	return c.client.interactions.Resolve(ctx, c.agentID, resolution)
}

func (c *memoryConversations) resolveNative(_ context.Context, resolution agentengine.InteractionResolution) error {
	resolution.ConversationKey = agentengine.ConversationKey(strings.TrimSpace(string(resolution.ConversationKey)))
	resolution.InteractionID = strings.TrimSpace(resolution.InteractionID)
	resolution.ResponderID = strings.TrimSpace(resolution.ResponderID)
	identity := memoryConversation{agentID: c.agentID, key: resolution.ConversationKey}
	c.client.mu.Lock()
	turn := c.client.active[identity]
	if turn == nil {
		c.client.mu.Unlock()
		return &agentengine.TurnError{Code: agentengine.ErrorInteractionNotFound, Message: "pending interaction was not found"}
	}
	pending := turn.interactions[resolution.InteractionID]
	if pending == nil || pending.resolving {
		c.client.mu.Unlock()
		return &agentengine.TurnError{Code: agentengine.ErrorInteractionNotFound, Message: "pending interaction was not found"}
	}
	pending.resolving = true
	delete(turn.interactions, resolution.InteractionID)
	c.client.resolutions = append(c.client.resolutions, Resolution{AgentID: c.agentID, Value: cloneResolution(resolution)})
	c.client.mu.Unlock()
	return nil
}

func (c *memoryConversations) admit(ctx context.Context, identity memoryConversation, request agentengine.TurnRequest) (*memoryTurn, *memoryCompletedTurn, *memoryTurn, *agentengine.TurnResult) {
	for {
		c.client.mu.Lock()
		turnKey := memoryTurnIdentity{memoryConversation: identity, id: request.ID}
		if completed, ok := c.client.completed[turnKey]; ok {
			if !sameMemoryRequest(completed.request, request) {
				c.client.mu.Unlock()
				result := failed(agentengine.ErrorInvalidRequest, "turn ID was already used with a different request")
				return nil, nil, nil, &result
			}
			copy := cloneMemoryCompleted(completed)
			c.client.mu.Unlock()
			return nil, &copy, nil, nil
		}
		if current := c.client.active[identity]; current != nil && current.request.ID == request.ID {
			if !sameMemoryRequest(current.request, request) {
				c.client.mu.Unlock()
				result := failed(agentengine.ErrorInvalidRequest, "turn ID is active with a different request")
				return nil, nil, nil, &result
			}
			c.client.mu.Unlock()
			return nil, nil, current, nil
		}
		control := c.client.controls[identity]
		current := c.client.active[identity]
		if control == nil && current == nil {
			turn := newMemoryTurn(ctx, request)
			c.client.active[identity] = turn
			c.client.mu.Unlock()
			return turn, nil, nil, nil
		}
		switch request.Admission {
		case agentengine.AdmissionRejectIfBusy:
			c.client.mu.Unlock()
			result := failed(agentengine.ErrorConversationBusy, "conversation already has an active turn or control operation")
			return nil, nil, nil, &result
		case agentengine.AdmissionWait:
			done := memoryControlDone(control, current)
			c.client.mu.Unlock()
			if err := waitMemory(ctx, done); err != nil {
				result := memoryContextResult(ctx, err)
				return nil, nil, nil, &result
			}
		case agentengine.AdmissionSupersede:
			if control != nil {
				done := control.done
				c.client.mu.Unlock()
				if err := waitMemory(ctx, done); err != nil {
					result := memoryContextResult(ctx, err)
					return nil, nil, nil, &result
				}
				continue
			}
			owned := &memoryControl{done: make(chan struct{})}
			c.client.controls[identity] = owned
			current.cancel()
			done := current.done
			c.client.mu.Unlock()
			if err := waitMemory(ctx, done); err != nil {
				c.endMemoryControl(identity, owned)
				result := memoryContextResult(ctx, err)
				return nil, nil, nil, &result
			}
			turn := newMemoryTurn(ctx, request)
			if !c.promoteMemoryControl(identity, owned, turn) {
				turn.cancel()
				result := failed(agentengine.ErrorConversationBusy, "conversation admission changed while superseding")
				return nil, nil, nil, &result
			}
			return turn, nil, nil, nil
		}
	}
}

func newMemoryTurn(ctx context.Context, request agentengine.TurnRequest) *memoryTurn {
	runCtx, cancel := context.WithCancel(ctx)
	return &memoryTurn{
		request:      cloneRequest(request),
		ctx:          runCtx,
		cancel:       cancel,
		done:         make(chan struct{}),
		interactions: make(map[string]*memoryInteraction),
		callIndex:    -1,
	}
}

func (c *memoryConversations) beginMemoryControl(identity memoryConversation) (*memoryControl, *memoryTurn, bool) {
	c.client.mu.Lock()
	defer c.client.mu.Unlock()
	if c.client.controls[identity] != nil {
		return nil, nil, false
	}
	control := &memoryControl{done: make(chan struct{})}
	c.client.controls[identity] = control
	active := c.client.active[identity]
	if active != nil {
		active.cancel()
	}
	return control, active, true
}

func (c *memoryConversations) promoteMemoryControl(identity memoryConversation, control *memoryControl, turn *memoryTurn) bool {
	c.client.mu.Lock()
	defer c.client.mu.Unlock()
	if c.client.controls[identity] != control || c.client.active[identity] != nil {
		return false
	}
	c.client.active[identity] = turn
	delete(c.client.controls, identity)
	close(control.done)
	return true
}

func (c *memoryConversations) endMemoryControl(identity memoryConversation, control *memoryControl) {
	c.client.mu.Lock()
	if c.client.controls[identity] == control {
		delete(c.client.controls, identity)
		close(control.done)
	}
	c.client.mu.Unlock()
}

func (c *memoryConversations) recordMemoryEvent(ctx context.Context, turn *memoryTurn, sink agentengine.EventSink, event agentengine.TurnEvent) error {
	c.client.mu.Lock()
	turn.sequence++
	event.TurnID = turn.request.ID
	event.Sequence = turn.sequence
	if event.Interaction != nil {
		interaction := *event.Interaction
		interaction.ID = strings.TrimSpace(interaction.ID)
		if interaction.ID != "" {
			turn.interactions[interaction.ID] = &memoryInteraction{request: interaction}
			c.client.interactions.Register(c.agentID, turn.request.ConversationKey, turn.request.ID, interaction, func(ctx context.Context, _ agentengine.InteractionRequest, resolution agentengine.InteractionResolution) *agentengine.TurnError {
				err := c.resolveNative(ctx, resolution)
				if err != nil {
					return &agentengine.TurnError{Code: agentengine.ErrorCodeOf(err), Message: err.Error()}
				}
				return nil
			}, nil)
			event.Interaction = &interaction
		}
	}
	turn.events = append(turn.events, cloneEvent(event))
	identity := memoryTurnIdentity{memoryConversation: memoryConversation{agentID: c.agentID, key: turn.request.ConversationKey}, id: turn.request.ID}
	if completed, ok := c.client.completed[identity]; ok {
		completed.events = append(completed.events, cloneEvent(event))
		c.client.completed[identity] = completed
	}
	if turn.callIndex >= 0 {
		c.client.calls[turn.callIndex].Events = append(c.client.calls[turn.callIndex].Events, cloneEvent(event))
	}
	c.client.mu.Unlock()
	if sink == nil {
		return nil
	}
	return sink.Emit(ctx, cloneEvent(event))
}

func (c *memoryConversations) completeMemoryTurn(identity memoryConversation, turn *memoryTurn, result agentengine.TurnResult) {
	c.client.mu.Lock()
	defer c.client.mu.Unlock()
	turn.result = cloneMemoryResult(result)
	if turn.callIndex >= 0 {
		c.client.calls[turn.callIndex].Result = cloneMemoryResult(result)
	}
	if c.client.active[identity] == turn {
		delete(c.client.active, identity)
	}
	if result.Dispatched {
		key := memoryTurnIdentity{memoryConversation: identity, id: turn.request.ID}
		c.client.completed[key] = memoryCompletedTurn{request: cloneRequest(turn.request), events: cloneEvents(turn.events), result: cloneMemoryResult(result)}
		c.client.completedOrder = append(c.client.completedOrder, key)
		if len(c.client.completedOrder) > maxMemoryCompletedTurns {
			oldest := c.client.completedOrder[0]
			c.client.completedOrder = c.client.completedOrder[1:]
			delete(c.client.completed, oldest)
		}
	}
	close(turn.done)
}

func replayMemoryEvents(ctx context.Context, sink agentengine.EventSink, events []agentengine.TurnEvent, result agentengine.TurnResult) agentengine.TurnResult {
	for _, event := range events {
		if sink != nil {
			if err := sink.Emit(ctx, cloneEvent(event)); err != nil {
				failedResult := failed(agentengine.ErrorRuntimeFailed, err.Error())
				failedResult.Dispatched = result.Dispatched
				return failedResult
			}
		}
	}
	return cloneMemoryResult(result)
}

func waitMemory(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func memoryControlDone(control *memoryControl, turn *memoryTurn) <-chan struct{} {
	if control != nil {
		return control.done
	}
	return turn.done
}

func memoryContextResult(ctx context.Context, err error) agentengine.TurnResult {
	if err == nil {
		err = ctx.Err()
	}
	return agentengine.TurnResult{Status: agentengine.TurnCanceled, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: err.Error()}}
}

func sameMemoryRequest(left, right agentengine.TurnRequest) bool {
	left.Admission = ""
	right.Admission = ""
	return reflect.DeepEqual(left, right)
}

func cloneMemoryCompleted(input memoryCompletedTurn) memoryCompletedTurn {
	input.request = cloneRequest(input.request)
	input.events = cloneEvents(input.events)
	input.result = cloneMemoryResult(input.result)
	return input
}

func cloneMemoryResult(input agentengine.TurnResult) agentengine.TurnResult {
	input.Files = cloneMemoryFiles(input.Files)
	if input.Interactions != nil {
		items := make([]agentengine.InteractionRequest, len(input.Interactions))
		for i, item := range input.Interactions {
			items[i] = interactionstate.Clone(item)
		}
		input.Interactions = items
	}
	if input.Error != nil {
		errorCopy := *input.Error
		input.Error = &errorCopy
	}
	return input
}

func cloneMemoryFiles(input []agentengine.OutputFile) []agentengine.OutputFile {
	if input == nil {
		return nil
	}
	output := make([]agentengine.OutputFile, len(input))
	for index, file := range input {
		output[index] = file
	}
	return output
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
	if spec.DesiredState == "" {
		spec.DesiredState = agentengine.AgentDesiredStateRunning
	}
	return spec
}

func failed(code agentengine.ErrorCode, message string) agentengine.TurnResult {
	return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: code, Message: message}}
}

func cloneAgent(input agentengine.Agent) agentengine.Agent {
	input.Spec = cloneSpec(input.Spec)
	if input.Status.Instructions != nil {
		status := *input.Status.Instructions
		input.Status.Instructions = &status
	}
	if input.Status.Memory != nil {
		status := *input.Status.Memory
		input.Status.Memory = &status
	}
	input.Spec.Runtime.Credentials = nil
	input.Spec.Model.APIKey = ""
	return input
}

func cloneSpec(input agentengine.AgentSpec) agentengine.AgentSpec {
	input.Skills = append([]string(nil), input.Skills...)
	input.Runtime.Credentials = cloneStringMap(input.Runtime.Credentials)
	input.Runtime.Options = cloneAnyMap(input.Runtime.Options)
	input.Model.Options = cloneAnyMap(input.Model.Options)
	input.Model.Headers = maps.Clone(input.Model.Headers)
	input.Model.Env = maps.Clone(input.Model.Env)
	if input.Memory != nil {
		memory := *input.Memory
		input.Memory = &memory
	}
	servers := input.MCPServers
	input.MCPServers = make(map[string]agentengine.MCPServerConfig, len(servers))
	for name, config := range servers {
		input.MCPServers[name] = agentengine.MCPServerConfig(cloneAnyMap(map[string]any(config)))
	}
	return input
}

func redactedModelView(spec agentengine.ModelSpec, apiKey string) agentengine.ModelView {
	viewSpec := spec
	viewSpec.APIKey = ""
	view := agentengine.ModelView{
		ModelSpec:       viewSpec,
		APIKeySet:       strings.TrimSpace(apiKey) != "",
		ProfileComplete: strings.TrimSpace(spec.ProviderID) != "" && strings.TrimSpace(spec.ModelID) != "",
	}
	runes := []rune(strings.TrimSpace(apiKey))
	if len(runes) >= 9 {
		view.APIKeyPreview = string(runes[:4]) + "..."
	}
	return view
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
	answers := input.Answers
	input.Answers = make(map[string]agentengine.InteractionAnswer, len(answers))
	for key, answer := range answers {
		answer.Values = append([]string(nil), answer.Values...)
		input.Answers[key] = answer
	}
	return input
}

func cloneCall(input TurnCall) TurnCall {
	input.Request = cloneRequest(input.Request)
	input.Events = cloneEvents(input.Events)
	return input
}

func cloneEvents(input []agentengine.TurnEvent) []agentengine.TurnEvent {
	output := make([]agentengine.TurnEvent, len(input))
	for index, event := range input {
		output[index] = cloneEvent(event)
	}
	return output
}

func cloneEvent(event agentengine.TurnEvent) agentengine.TurnEvent {
	event.Tool = cloneTool(event.Tool)
	event.Activity = cloneActivity(event.Activity)
	event.Interaction = cloneInteraction(event.Interaction)
	event.Output = cloneOutput(event.Output)
	return event
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

func cloneMCPServers(input map[string]agentengine.MCPServerConfig) map[string]agentengine.MCPServerConfig {
	if input == nil {
		return nil
	}
	out := make(map[string]agentengine.MCPServerConfig, len(input))
	for name, config := range input {
		out[name] = agentengine.MCPServerConfig(cloneAnyMap(map[string]any(config)))
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
var _ agentengine.AgentInterface = (*memoryAgents)(nil)
