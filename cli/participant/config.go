package participant

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
)

const feishuConfigAPIPath = "/api/v1/channels/feishu/config"

func (c cmd) runConfig(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet(
		c.Name()+" config",
		run.Program+" "+c.Name()+" config --channel feishu (--get|--set|--reload) [flags]",
		"Manage participant channel configuration.",
	)
	channelName := fs.String("channel", "feishu", "channel name; only feishu supports participant config")
	get := fs.Bool("get", false, "get masked channel config")
	set := fs.Bool("set", false, "set channel config")
	reload := fs.Bool("reload", false, "reload channel config")
	botID := fs.String("bot-id", "", "Feishu config key")
	appID := fs.String("app-id", "", "Feishu app id")
	adminOpenID := fs.String("admin-open-id", "", "Feishu admin open_id")
	secretFile := fs.String("app-secret-file", "", "read Feishu app secret from file")
	secretEnv := fs.String("app-secret-env", "", "read Feishu app secret from environment variable")
	secretStdin := fs.Bool("app-secret-stdin", false, "read Feishu app secret from stdin")
	noReload := fs.Bool("no-reload", false, "write config without reloading running server")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("%s config does not accept positional arguments", c.Name())
	}
	if normalizeChannel(*channelName) != "feishu" {
		return fmt.Errorf("%s config currently supports only --channel feishu", c.Name())
	}

	actions := 0
	for _, enabled := range []bool{*get, *set, *reload} {
		if enabled {
			actions++
		}
	}
	if actions != 1 {
		return fmt.Errorf("provide exactly one of --get, --set, or --reload")
	}

	switch {
	case *get:
		return c.runConfigGet(ctx, run, globals, *botID)
	case *set:
		return c.runConfigSet(ctx, run, globals, *botID, *appID, *adminOpenID, *secretFile, *secretEnv, *secretStdin, *noReload)
	case *reload:
		return c.runConfigReload(ctx, run, globals)
	default:
		return nil
	}
}

func (c cmd) runConfigGet(ctx context.Context, run *command.Context, globals command.GlobalOptions, botID string) error {
	id, err := requireConfigKey(botID)
	if err != nil {
		return err
	}
	values := url.Values{"bot_id": []string{id}}
	var resp apitypes.FeishuConfigResponse
	if err := run.APIClient(globals).DoJSON(ctx, http.MethodGet, feishuConfigAPIPath+"?"+values.Encode(), nil, &resp); err != nil {
		return err
	}
	return renderConfig(globals.Output, run.Stdout, resp)
}

func (c cmd) runConfigSet(ctx context.Context, run *command.Context, globals command.GlobalOptions, botID, appID, adminOpenID, secretFile, secretEnv string, secretStdin bool, noReload bool) error {
	id, err := requireConfigKey(botID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(appID) == "" {
		return fmt.Errorf("%s config --set requires --app-id", c.Name())
	}
	secret, err := readSecret(run.Stdin, secretFile, secretEnv, secretStdin)
	if err != nil {
		return err
	}
	reload := !noReload
	req := apitypes.FeishuConfigRequest{
		BotID:       id,
		AppID:       strings.TrimSpace(appID),
		AppSecret:   secret,
		AdminOpenID: strings.TrimSpace(adminOpenID),
		Reload:      &reload,
	}
	var resp apitypes.FeishuConfigResponse
	if err := run.APIClient(globals).DoJSON(ctx, http.MethodPut, feishuConfigAPIPath, req, &resp); err != nil {
		return err
	}
	return renderConfig(globals.Output, run.Stdout, resp)
}

func (c cmd) runConfigReload(ctx context.Context, run *command.Context, globals command.GlobalOptions) error {
	var resp apitypes.FeishuConfigReloadResponse
	if err := run.APIClient(globals).DoJSON(ctx, http.MethodPost, feishuConfigAPIPath, nil, &resp); err != nil {
		return err
	}
	return renderConfigReload(globals.Output, run.Stdout, resp)
}

func requireConfigKey(botID string) (string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return "", fmt.Errorf("--bot-id is required")
	}
	return botID, nil
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

func renderConfig(output string, w io.Writer, cfg apitypes.FeishuConfigResponse) error {
	if output == "json" {
		return command.WriteJSON(w, cfg)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "BOT_ID\tCONFIGURED\tAPP_ID\tAPP_SECRET\tADMIN_OPEN_ID\tRELOADED")
	fmt.Fprintf(tw, "%s\t%t\t%s\t%s\t%s\t%t\n", cfg.BotID, cfg.Configured, display(cfg.AppID), display(cfg.AppSecret), display(cfg.AdminOpenID), cfg.Reloaded)
	return tw.Flush()
}

func renderConfigReload(output string, w io.Writer, resp apitypes.FeishuConfigReloadResponse) error {
	if output == "json" {
		return command.WriteJSON(w, resp)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATUS\tFEISHU_BOTS")
	fmt.Fprintf(tw, "%s\t%s\n", display(resp.Status), display(strings.Join(resp.FeishuBots, ",")))
	return tw.Flush()
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
