package apps

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"csgclaw/cli/command"
	"csgclaw/internal/apiclient"
)

type cmd struct{}

func NewCmd() command.Command { return cmd{} }
func (cmd) Name() string      { return "app" }
func (cmd) Summary() string   { return "List and manage an Agent's Apps." }

func (c cmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	if len(args) == 0 || command.IsHelpArg(args[0]) {
		run.UsageCommandGroup(c, run.Program+" app <subcommand> [flags]", []string{"catalog                 List built-in App definitions", "list --global          List global App resources", "add --global --file FILE", "add --agent ID --file FILE  Bind a resource_id", "list --agent ID         List an Agent's Apps", "get --agent ID --id ID  Show one installation", "add --agent ID --file FILE", "update --agent ID --id ID --file FILE", "probe --agent ID --file FILE", "connect --agent ID --id ID", "disconnect --agent ID --id ID", "remove --agent ID --id ID"})
		return flag.ErrHelp
	}
	action := args[0]
	switch action {
	case "catalog", "list", "get", "add", "update", "probe", "connect", "disconnect", "remove":
	default:
		return fmt.Errorf("unknown app subcommand")
	}
	fs := run.NewFlagSet("app "+action, run.Program+" app "+action+" [flags]", c.Summary())
	caller := strings.TrimSpace(os.Getenv("CSGCLAW_CALLER_AGENT_ID"))
	agentID := fs.String("agent", caller, "Agent ID")
	global := fs.Bool("global", false, "Manage global App resources")
	installationID := fs.String("id", "", "installation ID; App definition ID for catalog")
	var file *string
	if action == "add" || action == "update" || action == "probe" {
		file = fs.String("file", "", "JSON request file; - reads standard input")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("app commands do not accept positional arguments")
	}
	*agentID = strings.TrimSpace(*agentID)
	*installationID = strings.TrimSpace(*installationID)
	if action != "catalog" && !*global && *agentID == "" {
		return fmt.Errorf("app %s requires --agent", action)
	}
	if action == "get" || action == "update" || action == "connect" || action == "disconnect" || action == "remove" {
		if *installationID == "" {
			return fmt.Errorf("app %s requires --id", action)
		}
	}
	if caller != "" && action != "catalog" && action != "list" && action != "get" {
		return fmt.Errorf("Change App settings in the UI: %s", settingsURL(globals.Endpoint, *agentID, *installationID))
	}
	if file != nil && strings.TrimSpace(*file) == "" {
		return fmt.Errorf("app %s requires --file (use - for standard input)", action)
	}
	if *global && caller != "" {
		return fmt.Errorf("Global App resources are managed in the UI")
	}
	if *global && (action == "connect" || action == "disconnect") {
		return fmt.Errorf("Connect an Agent binding; use update --global to enable or disable a resource")
	}
	client := run.APIClient(globals)
	collection := "/api/v1/agents/" + url.PathEscape(*agentID) + "/connectors"
	if *global {
		collection = "/api/v1/connectors/resources"
	}
	instance := collection + "/" + url.PathEscape(*installationID)
	var result map[string]any
	var err error
	switch action {
	case "catalog":
		path := "/api/v1/connectors/catalog"
		if *installationID != "" && caller == "" {
			path += "/" + url.PathEscape(*installationID)
		}
		err = client.GetJSON(ctx, path, &result)
		if err == nil && caller != "" && *installationID != "" {
			result, err = selectItem(result, "app_id", *installationID)
		}
	case "list":
		err = client.GetJSON(ctx, collection, &result)
	case "get":
		if caller == "" {
			err = client.GetJSON(ctx, instance, &result)
		} else {
			err = client.GetJSON(ctx, collection, &result)
			if err == nil {
				result, err = selectItem(result, "installation_id", *installationID)
			}
		}
	case "add", "update", "probe":
		input, readErr := readInput(run, *file)
		if readErr != nil {
			return readErr
		}
		path, method := collection, http.MethodPost
		if action == "update" {
			path, method = instance, http.MethodPatch
		}
		if action == "probe" {
			path = collection + ":probe"
			if *installationID != "" {
				input["installation_id"] = json.RawMessage(strconvJSON(*installationID))
			}
		}
		err = client.DoJSON(ctx, method, path, input, &result)
	case "connect", "disconnect":
		err = client.DoJSON(ctx, http.MethodPost, instance+"/"+action, map[string]any{}, &result)
	case "remove":
		err = client.DoJSON(ctx, http.MethodDelete, instance, nil, nil)
		if err == nil {
			return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{Command: "app", Action: "remove", Status: "removed", ID: *installationID, Message: "App removed."})
		}
	}
	if err != nil {
		return err
	}
	if caller != "" && action != "catalog" {
		result["settings_url"] = settingsURL(globals.Endpoint, *agentID, *installationID)
	}
	return render(globals.Output, run.Stdout, result)
}

func strconvJSON(value string) []byte { encoded, _ := json.Marshal(value); return encoded }

func readInput(run *command.Context, file string) (map[string]json.RawMessage, error) {
	var reader io.Reader = run.Stdin
	if file != "-" {
		opened, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("cannot open App request file")
		}
		defer opened.Close()
		reader = opened
	}
	if reader == nil {
		return nil, fmt.Errorf("App JSON input is required")
	}
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, fmt.Errorf("App JSON input must be at most 1 MiB")
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(data, &value) != nil || value == nil {
		return nil, fmt.Errorf("App input must be one JSON object")
	}
	return value, nil
}

func selectItem(result map[string]any, key, id string) (map[string]any, error) {
	items, _ := result["items"].([]any)
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok && item[key] == id {
			return item, nil
		}
	}
	return nil, fmt.Errorf("App not found")
}

func settingsURL(endpoint, agentID, id string) string {
	if endpoint == "" {
		endpoint = apiclient.DefaultAPIBaseURL()
	}
	values := url.Values{"tab": []string{"connectors"}}
	if id != "" {
		values.Set("connector", id)
	}
	return strings.TrimRight(endpoint, "/") + "/#/agents/" + url.PathEscape(agentID) + "?" + values.Encode()
}

func render(output string, w io.Writer, value map[string]any) error {
	format, err := command.NormalizeOutput(output)
	if err != nil {
		return err
	}
	if format == "json" {
		return command.WriteJSON(w, value)
	}
	if connected, ok := value["connected"].(bool); ok {
		tools, _ := value["tools"].([]any)
		_, err := fmt.Fprintf(w, "Connected: %t\nTools: %d\n", connected, len(tools))
		return err
	}
	items := []map[string]any{}
	if raw, ok := value["items"].([]any); ok {
		for _, row := range raw {
			if item, ok := row.(map[string]any); ok {
				items = append(items, item)
			}
		}
	} else {
		items = append(items, value)
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tAPP\tNAME\tSTATUS")
	text := func(row map[string]any, key string) string {
		value, _ := row[key].(string)
		return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(value)
	}
	for _, item := range items {
		id := text(item, "installation_id")
		if id == "" {
			id = text(item, "app_id")
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", id, text(item, "app_id"), text(item, "name"), text(item, "status"))
	}
	if err := table.Flush(); err != nil {
		return err
	}
	if link, ok := value["settings_url"].(string); ok {
		_, err = fmt.Fprintln(w, link)
	}
	return err
}
