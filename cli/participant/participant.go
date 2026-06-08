package participant

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/agent"
	participantpkg "csgclaw/internal/participant"
)

type cmd struct {
	name string
}

func NewCmd() command.Command {
	return cmd{name: "participant"}
}

func NewAliasCmd(name string) command.Command {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "pt"
	}
	return cmd{name: name}
}

func (c cmd) Name() string {
	return c.name
}

func (cmd) Summary() string {
	return "Manage channel participants."
}

func (c cmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	if len(args) == 0 {
		c.usage(run)
		return flag.ErrHelp
	}
	if command.IsHelpArg(args[0]) {
		c.usage(run)
		return flag.ErrHelp
	}

	switch args[0] {
	case "list":
		return c.runList(ctx, run, args[1:], globals)
	case "create":
		return c.runCreate(ctx, run, args[1:], globals)
	case "delete":
		return c.runDelete(ctx, run, args[1:], globals)
	case "config":
		return c.runConfig(ctx, run, args[1:], globals)
	default:
		c.usage(run)
		return fmt.Errorf("unknown %s subcommand %q", c.Name(), args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	subcommands := []string{
		"list               List participants",
		"create             Create a participant",
		"delete <id>        Delete a participant",
		"config             Manage participant channel config",
	}
	run.UsageCommandGroup(c, run.Program+" "+c.Name()+" <subcommand> [flags]", subcommands)
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet(c.Name()+" list", run.Program+" "+c.Name()+" list [flags]", "List participants.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	participantType := fs.String("type", "", "participant type: human, agent, or notification")
	agentID := fs.String("agent-id", "", "filter by bound agent id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("%s list does not accept positional arguments", c.Name())
	}

	items, err := run.APIClient(globals).ListParticipants(ctx, *channelName, *participantType, *agentID)
	if err != nil {
		return err
	}
	return command.RenderParticipants(globals.Output, run.Stdout, items)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet(c.Name()+" create", run.Program+" "+c.Name()+" create [flags]", "Create a participant.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	id := fs.String("id", "", "participant id")
	name := fs.String("name", "", "participant display name")
	description := fs.String("description", "", "agent description for bind create and participant metadata")
	participantType := fs.String("type", participantpkg.TypeAgent, "participant type: human, agent, or notification")
	channelUserRef := fs.String("channel-user-ref", "", "channel user identity such as local user id or Feishu open_id")
	channelUserKind := fs.String("channel-user-kind", "", "channel user identity kind such as local_user_id or open_id")
	channelAppRef := fs.String("channel-app-ref", "", "channel app/config reference such as Feishu app_id")
	bindMode := fs.String("bind", participantpkg.BindingModeNone, "agent binding mode: create, reuse, or none")
	agentID := fs.String("agent-id", "", "agent id for bind reuse, or optional id for bind create")
	role := fs.String("role", "", "agent role for bind create")
	runtimeKind := fs.String("runtime", "", "agent runtime kind for bind create")
	image := fs.String("image", "", "agent image for bind create")
	fromTemplate := fs.String("from-template", "", "hub template for bind create")
	modelID := fs.String("model-id", "", "agent model id for bind create")
	var envValues envFlag
	fs.Var(&envValues, "env", "agent image environment variable as KEY=VALUE (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("%s create does not accept positional arguments", c.Name())
	}
	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("%s create requires --name", c.Name())
	}
	envMap, err := parseEnvAssignments(envValues)
	if err != nil {
		return err
	}

	req := participantpkg.CreateRequest{
		ID:            *id,
		Channel:       *channelName,
		Type:          *participantType,
		Name:          *name,
		ChannelAppRef: *channelAppRef,
		ChannelUser: participantpkg.ChannelUserSpec{
			Ref:  *channelUserRef,
			Kind: *channelUserKind,
		},
		AgentBinding: participantpkg.AgentBindingSpec{
			Mode:    *bindMode,
			AgentID: *agentID,
		},
	}
	if strings.TrimSpace(*description) != "" {
		req.Metadata = map[string]any{"description": strings.TrimSpace(*description)}
	}
	if strings.EqualFold(strings.TrimSpace(*bindMode), participantpkg.BindingModeCreate) {
		spec := agent.CreateAgentSpec{
			ID:           *agentID,
			Name:         *name,
			Description:  *description,
			Role:         *role,
			RuntimeKind:  *runtimeKind,
			Image:        *image,
			FromTemplate: *fromTemplate,
		}
		if strings.TrimSpace(*modelID) != "" {
			spec.AgentProfile.ModelID = *modelID
		}
		if len(envMap) > 0 {
			spec.AgentProfile.Env = envMap
		}
		req.AgentBinding.Agent = &spec
	}

	created, err := run.APIClient(globals).CreateParticipant(ctx, req)
	if err != nil {
		return err
	}
	return command.RenderParticipants(globals.Output, run.Stdout, []participantpkg.Participant{created})
}

type envFlag []string

func (e *envFlag) String() string {
	return strings.Join(*e, ",")
}

func (e *envFlag) Set(value string) error {
	*e = append(*e, value)
	return nil
}

func parseEnvAssignments(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		key, value, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --env %q: expected KEY=VALUE", raw)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate --env key %q", key)
		}
		out[key] = value
	}
	return out, nil
}

func (c cmd) runDelete(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet(c.Name()+" delete", run.Program+" "+c.Name()+" delete <id> [flags]", "Delete a participant.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	deleteAgent := fs.String("delete-agent", "", "agent cleanup mode; supported: if_unreferenced")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("%s delete requires exactly one id", c.Name())
	}
	if err := run.APIClient(globals).DeleteParticipant(ctx, *channelName, rest[0], *deleteAgent); err != nil {
		return err
	}
	return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
		Command: c.Name(),
		Action:  "delete",
		Status:  "deleted",
		ID:      rest[0],
		Channel: *channelName,
		Message: fmt.Sprintf("deleted %s participant %s", *channelName, rest[0]),
	})
}
