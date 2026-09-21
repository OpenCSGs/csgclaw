package dsh

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"csgclaw/internal/mcpschema"
)

type acpNameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type acpMCPServer struct {
	Type    string         `json:"type,omitempty"`
	Name    string         `json:"name"`
	Command string         `json:"command,omitempty"`
	Args    []string       `json:"args,omitempty"`
	Env     []acpNameValue `json:"env,omitempty"`
	URL     string         `json:"url,omitempty"`
	Headers []acpNameValue `json:"headers,omitempty"`
}

func (s acpMCPServer) MarshalJSON() ([]byte, error) {
	if strings.EqualFold(strings.TrimSpace(s.Type), "http") {
		headers := s.Headers
		if headers == nil {
			headers = []acpNameValue{}
		}
		return json.Marshal(struct {
			Type    string         `json:"type"`
			Name    string         `json:"name"`
			URL     string         `json:"url"`
			Headers []acpNameValue `json:"headers"`
		}{Type: "http", Name: s.Name, URL: s.URL, Headers: headers})
	}
	args := s.Args
	if args == nil {
		args = []string{}
	}
	env := s.Env
	if env == nil {
		env = []acpNameValue{}
	}
	return json.Marshal(struct {
		Name    string         `json:"name"`
		Command string         `json:"command"`
		Args    []string       `json:"args"`
		Env     []acpNameValue `json:"env"`
	}{Name: s.Name, Command: s.Command, Args: args, Env: env})
}

func buildACPMCPServers(raw map[string]any) ([]acpMCPServer, error) {
	servers, err := mcpschema.NormalizeMCPServers(raw)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]acpMCPServer, 0, len(names))
	for _, name := range names {
		entry := servers[name].(map[string]any)
		command, _ := entry["command"].(string)
		urlText, _ := entry["url"].(string)
		if command != "" {
			if !filepath.IsAbs(command) {
				return nil, fmt.Errorf("mcpServers.%s.command must be an absolute path for DSH ACP", name)
			}
			server := acpMCPServer{Name: name, Command: command}
			server.Args = stringSlice(entry["args"])
			server.Env = nameValues(entry["env"])
			out = append(out, server)
			continue
		}
		parsed, parseErr := url.Parse(urlText)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("mcpServers.%s.url must be an absolute HTTP(S) URL for DSH ACP", name)
		}
		transport, _ := entry["transport"].(string)
		if transport != "" && !strings.EqualFold(strings.TrimSpace(transport), "streamable-http") && !strings.EqualFold(strings.TrimSpace(transport), "http") {
			return nil, fmt.Errorf("mcpServers.%s.transport %q is not supported by DSH ACP", name, transport)
		}
		out = append(out, acpMCPServer{Type: "http", Name: name, URL: urlText, Headers: nameValues(entry["headers"])})
	}
	return out, nil
}

func stringSlice(raw any) []string {
	items, _ := raw.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func nameValues(raw any) []acpNameValue {
	values, _ := raw.(map[string]any)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]acpNameValue, 0, len(keys))
	for _, key := range keys {
		if value, ok := values[key].(string); ok {
			out = append(out, acpNameValue{Name: key, Value: value})
		}
	}
	return out
}
