package relay

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"csgclaw/cli/command"
	"csgclaw/internal/config"
	"csgclaw/internal/relay"
	"github.com/google/uuid"
)

type cmd struct{}

func NewCmd() command.Command { return cmd{} }
func (cmd) Name() string      { return "relay" }
func (cmd) Summary() string   { return "Publish local agents and call remote agents through CSGRelay." }
func (c cmd) Run(ctx context.Context, run *command.Context, args []string, g command.GlobalOptions) error {
	if len(args) == 0 || command.IsHelpArg(args[0]) {
		run.UsageCommandGroup(c, run.Program+" relay <subcommand> [flags]", []string{"publish       Register a local agent and keep its outbound executor online", "list          Discover remote agents", "get           Get a remote agent", "unpublish     Unpublish a remote agent", "call          Invoke a remote agent (JSON or raw SSE)", "response      Query an invocation", "cancel        Request cancellation"})
		return flag.ErrHelp
	}
	action := args[0]
	switch action {
	case "publish", "list", "get", "unpublish", "call", "response", "cancel":
	default:
		return fmt.Errorf("unknown relay subcommand %q", action)
	}
	fs := run.NewFlagSet("relay "+action, run.Program+" relay "+action+" [flags]", c.Summary())
	endpoint := fs.String("relay", envOr("CSGCLAW_RELAY_URL", "http://127.0.0.1:28080"), "relay base URL (CSGCLAW_RELAY_URL)")
	var agentID, name, description, visibility, key, state, input, previous, id, scope, status, cursor string
	var stream bool
	var limit int
	var timeout time.Duration
	switch action {
	case "publish":
		fs.StringVar(&agentID, "agent", "", "local agent ID (required)")
		fs.StringVar(&name, "name", "", "published name (defaults to local name)")
		fs.StringVar(&description, "description", "", "published description (defaults to local description)")
		fs.StringVar(&visibility, "visibility", "private", "private or public")
		fs.StringVar(&key, "key", "", "publication idempotency key (generated if omitted)")
		fs.StringVar(&state, "state", "", "executor state file (defaults under csgclaw relay directory)")
	case "list":
		fs.StringVar(&scope, "scope", "accessible", "public, mine or accessible")
		fs.StringVar(&status, "status", "", "online, offline or unhealthy")
		fs.IntVar(&limit, "limit", 50, "page size (1-100)")
		fs.StringVar(&cursor, "cursor", "", "opaque pagination cursor")
	case "get", "unpublish":
		fs.StringVar(&id, "id", "", "relay agent ID (required)")
	case "call":
		fs.StringVar(&id, "agent", "", "relay agent ID (required)")
		fs.StringVar(&input, "input", "", "text input (required)")
		fs.StringVar(&previous, "previous-response", "", "completed response ID for continuation")
		fs.StringVar(&key, "key", "", "invocation idempotency key (generated if omitted)")
		fs.BoolVar(&stream, "stream", false, "write raw SSE events to stdout")
		fs.DurationVar(&timeout, "timeout", 6*time.Minute, "client wait timeout (disconnect does not cancel remote execution)")
	case "response", "cancel":
		fs.StringVar(&id, "id", "", "response ID (required)")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("relay %s does not accept positional arguments", action)
	}
	client, err := relay.NewClient(*endpoint, os.Getenv("CSGCLAW_RELAY_TOKEN"), nil)
	if err != nil {
		return err
	}
	if key == "" {
		key = uuid.NewString()
	}
	switch action {
	case "publish":
		if agentID == "" {
			return fmt.Errorf("--agent is required")
		}
		if visibility != "public" && visibility != "private" {
			return fmt.Errorf("--visibility must be public or private")
		}
		local := run.APIClient(g)
		agent, err := local.GetAgent(ctx, agentID)
		if err != nil {
			return err
		}
		if agent.Runtime.SandboxEnabled || (agent.Runtime.Name != "codex" && agent.Runtime.Name != "dsh") {
			return fmt.Errorf("relay publishing currently requires a host codex or dsh agent supported by the Session API")
		}
		if name == "" {
			name = agent.Name
		}
		if description == "" {
			description = agent.Description
		}
		publication, err := client.Publish(ctx, key, relay.PublishRequest{Name: name, Description: description, Visibility: visibility, Executor: "csgclaw", SourceAgentID: agent.ID})
		if err != nil {
			return err
		}
		if publication.Agent.ID == "" {
			return fmt.Errorf("relay returned no agent ID")
		}
		if state == "" {
			root, err := config.DefaultDomainDir("relay")
			if err != nil {
				return err
			}
			state = filepath.Join(root, fmt.Sprintf("%x.json", sha256.Sum256([]byte(*endpoint+"\x00"+publication.Agent.ID))))
		}
		if err = command.WriteJSON(run.Stdout, publication); err != nil {
			return err
		}
		executor := &relay.Executor{Client: client, AgentID: publication.Agent.ID, StatePath: state, Handler: &relay.LocalExecutor{Client: local, AgentID: agent.ID, Scope: *endpoint + "\x00" + publication.Agent.ID}, OnReconnect: func() { fmt.Fprintln(run.Stderr, "Relay executor disconnected; reconnecting.") }}
		return executor.Run(ctx)
	case "list":
		if scope != "public" && scope != "mine" && scope != "accessible" {
			return fmt.Errorf("invalid scope")
		}
		if limit < 1 || limit > 100 {
			return fmt.Errorf("limit must be 1-100")
		}
		q := url.Values{"scope": {scope}, "limit": {strconv.Itoa(limit)}}
		if status != "" {
			q.Set("status", status)
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var list relay.AgentList
		err = client.Request(ctx, http.MethodGet, "/v1/agents?"+q.Encode(), "", nil, &list)
		if err == nil {
			err = command.WriteJSON(run.Stdout, list)
		}
		return err
	case "call":
		if id == "" || input == "" {
			return fmt.Errorf("--agent and --input are required")
		}
		if timeout <= 0 {
			return fmt.Errorf("--timeout must be positive")
		}
		body, _ := json.Marshal(input)
		req := relay.CallRequest{Model: id, Input: body, PreviousResponseID: previous}
		fmt.Fprintf(run.Stderr, "Relay invocation idempotency key: %s\n", key)
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if stream {
			return client.Stream(ctx, key, req, run.Stdout)
		}
		response, err := client.Call(ctx, key, req)
		if err != nil {
			return err
		}
		if err = command.WriteJSON(run.Stdout, response); err != nil {
			return err
		}
		if response.Status == "failed" || response.Status == "cancelled" {
			return fmt.Errorf("remote invocation %s: %s", response.ID, response.Status)
		}
		return nil
	default:
		if id == "" {
			return fmt.Errorf("--id is required")
		}
		path := "/v1/agents/" + url.PathEscape(id)
		method := http.MethodGet
		if action == "unpublish" {
			method = http.MethodDelete
		}
		if action == "response" || action == "cancel" {
			path = "/v1/responses/" + url.PathEscape(id)
		}
		var body any
		if action == "cancel" {
			method = http.MethodPost
			path += "/cancel"
			body = map[string]any{}
		}
		var result json.RawMessage
		var out any = &result
		if action == "unpublish" {
			out = nil
		}
		err = client.Request(ctx, method, path, "", body, out)
		if err != nil {
			return err
		}
		if out == nil {
			return nil
		}
		return command.WriteJSON(run.Stdout, result)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
