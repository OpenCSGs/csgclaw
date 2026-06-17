package participant

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"csgclaw/cli/command"
	participantpkg "csgclaw/internal/participant"
	"csgclaw/internal/participant/feishubind"
)

func (c cmd) runBind(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet(
		c.Name()+" bind",
		run.Program+" "+c.Name()+" bind --channel feishu --feishu-kind (human|bot) [flags]",
		"Bind a channel identity to a participant.",
	)
	channelName := fs.String("channel", "feishu", "channel name; only feishu is supported")
	feishuKind := fs.String("feishu-kind", "", "Feishu identity kind: human or bot")
	agentRef := fs.String("agent", "", "agent name or id for Feishu bot binding")
	name := fs.String("name", "", "participant display name for Feishu human binding")
	admin := fs.Bool("admin", false, "bind the Feishu admin human participant")
	openID := fs.String("open-id", "", "Feishu human open_id")
	appID := fs.String("app-id", "", "Feishu app id for bot binding")
	secretFile := fs.String("app-secret-file", "", "read Feishu app secret from file")
	secretEnv := fs.String("app-secret-env", "", "read Feishu app secret from environment variable")
	secretStdin := fs.Bool("app-secret-stdin", false, "read Feishu app secret from stdin")
	restart := fs.Bool("restart", false, "recreate worker after bot config is saved; manager returns restart_status=manager_restart_required")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("%s bind does not accept positional arguments", c.Name())
	}
	if normalizeChannel(*channelName) != participantpkg.ChannelFeishu {
		return fmt.Errorf("%s bind currently supports only --channel feishu", c.Name())
	}
	kind := strings.ToLower(strings.TrimSpace(*feishuKind))
	switch kind {
	case "human":
		return c.runBindFeishuHuman(ctx, run, globals, *admin, *openID, *name)
	case "bot":
		return c.runBindFeishuBot(ctx, run, globals, *agentRef, *appID, *secretFile, *secretEnv, *secretStdin, *restart)
	default:
		return fmt.Errorf("--feishu-kind must be one of %q or %q", "human", "bot")
	}
}

func (c cmd) runBindFeishuHuman(ctx context.Context, run *command.Context, globals command.GlobalOptions, admin bool, openID, name string) error {
	if !admin {
		return fmt.Errorf("%s bind --feishu-kind human currently requires --admin", c.Name())
	}
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return fmt.Errorf("%s bind --feishu-kind human requires --open-id", c.Name())
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "admin"
	}
	client := run.APIClient(globals)
	result, err := feishubind.BindAdminHuman(ctx, client, openID, name)
	if err != nil {
		return err
	}
	return renderBindResult(globals.Output, run.Stdout, result)
}

func (c cmd) runBindFeishuBot(ctx context.Context, run *command.Context, globals command.GlobalOptions, agentRef, appID, secretFile, secretEnv string, secretStdin bool, restart bool) error {
	agentRef = strings.TrimSpace(agentRef)
	if agentRef == "" {
		return fmt.Errorf("%s bind --feishu-kind bot requires --agent", c.Name())
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return fmt.Errorf("%s bind --feishu-kind bot requires --app-id", c.Name())
	}
	appSecret, err := readSecret(run.Stdin, secretFile, secretEnv, secretStdin)
	if err != nil {
		return err
	}
	client := run.APIClient(globals)
	result, err := feishubind.BindBot(ctx, client, agentRef, appID, appSecret, restart)
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(run.Stderr, "warning:", warning)
	}
	if result.RestartStatus == "recreate_failed" {
		fmt.Fprintf(run.Stderr, "pt bind failed at recreate: agent_id=%s participant_id=%s error=%s\n", result.AgentID, result.ParticipantID, result.RestartError)
	}
	return renderBindResult(globals.Output, run.Stdout, result)
}

func normalizeChannel(channelName string) string {
	return strings.ToLower(strings.TrimSpace(channelName))
}

func display(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func readSecret(stdin io.Reader, filePath, envName string, fromStdin bool) (string, error) {
	count := 0
	if strings.TrimSpace(filePath) != "" {
		count++
	}
	if strings.TrimSpace(envName) != "" {
		count++
	}
	if fromStdin {
		count++
	}
	if count != 1 {
		return "", fmt.Errorf("provide exactly one of --app-secret-file, --app-secret-env, or --app-secret-stdin")
	}

	var secret string
	switch {
	case strings.TrimSpace(filePath) != "":
		data, err := os.ReadFile(strings.TrimSpace(filePath))
		if err != nil {
			return "", fmt.Errorf("read app secret file: %w", err)
		}
		secret = string(data)
	case strings.TrimSpace(envName) != "":
		value, ok := os.LookupEnv(strings.TrimSpace(envName))
		if !ok {
			return "", fmt.Errorf("environment variable %s is not set", strings.TrimSpace(envName))
		}
		secret = value
	case fromStdin:
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read app secret from stdin: %w", err)
		}
		secret = string(data)
	}

	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", fmt.Errorf("app secret is empty")
	}
	return secret, nil
}

func renderBindResult(output string, w io.Writer, result feishubind.Result) error {
	if output == "json" {
		return command.WriteJSON(w, result)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATUS\tCHANNEL\tTYPE\tPARTICIPANT_ID\tAGENT_ID\tCONFIG_SAVED\tRESTART\tRESTART_ERROR")
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%t\t%s\t%s\n",
		display(result.Status),
		display(result.Channel),
		display(result.ParticipantType),
		display(result.ParticipantID),
		display(result.AgentID),
		result.ConfigSaved,
		display(result.RestartStatus),
		display(result.RestartError),
	)
	return tw.Flush()
}
