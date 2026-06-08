package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/hub"
	"csgclaw/internal/upgrade"
)

func (h *Handler) resolveConfigPath() (string, error) {
	path := strings.TrimSpace(h.configPath)
	if path == "" {
		return config.DefaultPath()
	}
	return path, nil
}

func (h *Handler) handleConfigSettings(w http.ResponseWriter, r *http.Request) {
	path, err := h.resolveConfigPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg, _, err := h.loadBootstrapConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, configSettingsView(path, cfg))
	case http.MethodPut:
		var req apitypes.UpdateConfigSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		cfg, path, err := h.loadBootstrapConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		previousManager := cfg.Bootstrap.ResolvedDefaultManagerTemplate()
		previousWorker := cfg.Bootstrap.ResolvedDefaultWorkerTemplate()
		accessToken := strings.TrimSpace(req.AccessToken)
		if accessToken == "" {
			accessToken = cfg.Server.AccessToken
		}
		cfg, err = config.ApplyUserSettings(cfg, config.UserSettings{
			ListenAddr:             req.ListenAddr,
			AdvertiseBaseURL:       req.AdvertiseBaseURL,
			AccessToken:            accessToken,
			ShowUpgrade:            req.ShowUpgrade,
			SandboxProvider:        req.SandboxProvider,
			DefaultManagerTemplate: req.DefaultManagerTemplate,
			DefaultWorkerTemplate:  req.DefaultWorkerTemplate,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var bootstrapDefaults *hub.BootstrapDefaults
		if h.hub != nil {
			defaults, err := hub.ResolveBootstrapDefaults(r.Context(), cfg.Bootstrap, h.hub)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			bootstrapDefaults = &defaults
		}
		if err := cfg.Save(path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if h.svc != nil && bootstrapDefaults != nil {
			managerChanged := cfg.Bootstrap.ResolvedDefaultManagerTemplate() != previousManager
			workerChanged := cfg.Bootstrap.ResolvedDefaultWorkerTemplate() != previousWorker
			if managerChanged || workerChanged {
				if err := h.svc.SetGatewayRuntime(bootstrapDefaults.ManagerRuntimeKind, bootstrapDefaults.ManagerImage); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
		}
		writeJSON(w, http.StatusOK, configSettingsView(path, cfg))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func configSettingsView(path string, cfg config.Config) apitypes.ConfigSettingsResponse {
	settings := config.UserSettingsFromConfig(cfg)
	token := strings.TrimSpace(settings.AccessToken)
	effective := agent.ResolveManagerBaseURL(cfg.Server)
	if effective == "" {
		effective = config.ResolveAdvertiseBaseURL(cfg.Server)
	}
	return apitypes.ConfigSettingsResponse{
		Path:                      path,
		ListenAddr:                settings.ListenAddr,
		AdvertiseBaseURL:          settings.AdvertiseBaseURL,
		AdvertiseBaseURLEffective: effective,
		AccessTokenSet:            token != "",
		AccessTokenPreview:        config.AccessTokenPreview(token),
		ShowUpgrade:               settings.ShowUpgrade,
		SandboxProvider:           settings.SandboxProvider,
		SupportedSandboxProviders: settings.SupportedSandboxProvider,
		DefaultManagerTemplate:    settings.DefaultManagerTemplate,
		DefaultWorkerTemplate:     settings.DefaultWorkerTemplate,
	}
}

func (h *Handler) handleConfigFile(w http.ResponseWriter, r *http.Request) {
	path, err := h.resolveConfigPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		data, err := config.ReadRawFile(path)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "config not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusOK, apitypes.ConfigFileResponse{
			Path:    path,
			Content: string(data),
		})
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			http.Error(w, fmt.Sprintf("read request: %v", err), http.StatusBadRequest)
			return
		}
		var req apitypes.UpdateConfigRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		if err := config.WriteRawFile(path, []byte(req.Content)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, apitypes.ConfigFileResponse{
			Path:    path,
			Content: req.Content,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleConfigApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	configPath, err := h.resolveConfigPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apply := h.configRestartApply
	if apply == nil {
		apply = upgrade.StartRestartHelper
	}
	if err := apply(upgrade.RestartHelperOptions{ConfigPath: configPath}); err != nil {
		http.Error(w, fmt.Sprintf("start restart helper: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, apitypes.ConfigActionResponse{
		Status:  "accepted",
		Message: "restart helper started",
	})
}

func (h *Handler) handleConfigRestartStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	configPath, err := h.resolveConfigPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	record, err := upgrade.ConsumeRestartStatus(configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read restart helper status: %v", err), http.StatusInternalServerError)
		return
	}

	resp := apitypes.ConfigRestartStatusResponse{}
	switch record.Status {
	case upgrade.ApplyStatusManualRestartRequired:
		resp.ManualRestartRequired = true
		resp.Message = record.Message
	case upgrade.ApplyStatusFailed:
		resp.LastError = record.Message
	default:
		if record.Message != "" {
			resp.LastError = record.Message
		}
	}
	if resp.ManualRestartRequired || resp.LastError != "" || resp.Message != "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
