package api

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/auth"
	"csgclaw/internal/skill/catalog"
	"csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
	"csgclaw/internal/skill/remote"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Resolve site and credential from ONE stored login. Never send a candidate-
// supplied URL a credential, including when the user switched sites mid-turn.
type skillSite struct{ api, web, token string }

func loggedInSkillSite() (skillSite, error) {
	record, ok, err := auth.Default().Store.Load()
	if err != nil {
		return skillSite{}, err
	}
	status := record.Status()
	if !ok || !status.Authenticated {
		return skillSite{}, remote.ErrAgenticHubSignInRequired
	}
	env := openCSGEnvironmentFromStatus(status)
	base := strings.TrimRight(status.BaseURL, "/")
	if base == "" {
		base = env.CSGHubBaseURL
	}
	return skillSite{base, env.OpenCSGBaseURL, record.Tokens.AccessToken}, nil
}

func globalSkillCandidates(ctx context.Context, query, source string) ([]catalog.Candidate, error) {
	root, err := skilllocal.SkillsRoot()
	if err != nil {
		return nil, err
	}
	items, err := listSkills(root)
	if err != nil {
		return nil, err
	}
	out := make([]catalog.Candidate, 0, len(items))
	var current skillSite
	if source == catalog.Site {
		current, err = loggedInSkillSite()
		if err != nil {
			return nil, nil
		} // the site lookup reports login_required
	}
	for _, item := range items {
		c := catalog.Candidate{Key: item.Name, Name: item.Name, Description: item.Description, Source: catalog.Global}
		if rec := item.Install; rec != nil {
			c.Repository = rec.RemotePath
			c.Version = rec.Version
			c.Ref = rec.Ref
			c.URL = rec.DisplayURL()
		}
		if !matchesLibraryOrigin(source, current.web, item.Install) {
			continue
		}
		out = append(out, c)
	}
	matches := catalog.Match(out, query)
	if len(matches) == 0 {
		return nil, ctx.Err()
	}
	// Only hash the shortlisted entries. Refresh their visible metadata and
	// origin under the same source lock used when preparing an installation.
	out = nil
	err = installstate.ReadSnapshot(root, func(records map[string]installstate.Record) error {
		for _, c := range matches {
			b, err := skilllocal.ResolveLibraryBundle(root, c.Name, records[c.Name])
			if err != nil {
				return err
			}
			if !matchesLibraryOrigin(source, current.web, &b.Origin) {
				continue
			}
			c.Description, c.Version, err = b.Metadata()
			if err != nil {
				return err
			}
			c.URL, c.Ref = b.Origin.DisplayURL(), b.Origin.Ref
			c.Repository = b.Origin.RemotePath
			c.Selection, err = b.Selection(ctx, c.Name)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return nil
	})
	return catalog.Match(out, query), err
}

func matchesLibraryOrigin(source, web string, rec *installstate.Record) bool {
	switch source {
	case catalog.Site:
		if rec == nil || rec.SourceKind() != installstate.SourceMarket {
			return false
		}
		key := rec.RemotePath
		if key == "" {
			// Older market records have the site page URL but no remote_path.
			var found bool
			key, found = strings.CutPrefix(rec.DisplayURL(), strings.TrimRight(web, "/")+"/skills/")
			if !found || !catalog.IsRepositoryKey(key) {
				return false
			}
		}
		expected, err := remote.AgenticHubSkillWebURL(web, key)
		return err == nil && expected == rec.DisplayURL()
	case catalog.Public:
		return rec != nil && catalog.ValidPublicURL(rec.DisplayURL())
	default:
		return true
	}
}

func siteSkillCandidates(ctx context.Context, query string) ([]catalog.Candidate, error) {
	site, err := loggedInSkillSite()
	if err != nil {
		return nil, err
	}
	if catalog.IsRepositoryKey(query) {
		item, found, err := remote.FindAgenticHubSkill(ctx, site.api, site.token, query)
		if err != nil || !found {
			return nil, err
		}
		link, err := remote.AgenticHubSkillWebURL(site.web, query)
		if err != nil {
			return nil, err
		}
		return []catalog.Candidate{{Source: catalog.Site, Site: site.api, Key: query, Name: item.Name, URL: link, Ref: item.Ref, Version: item.Version, Description: item.Description}}, nil
	}
	list, err := remote.ListAgenticHubSkills(ctx, site.api, site.token, remote.AgenticHubSkillListOptions{Page: 1, Per: catalog.Limit, Search: query})
	if err != nil {
		return nil, err
	}
	var out []catalog.Candidate
	for _, item := range list.Items {
		link, err := remote.AgenticHubSkillWebURL(site.web, item.RemotePath)
		if err != nil {
			return nil, err
		}
		out = append(out, catalog.Candidate{Source: catalog.Site, Site: site.api, Name: item.Name, Key: item.RemotePath, Ref: item.Ref, URL: link, Description: item.Description, Version: item.Version})
	}
	return out, nil
}

func (h *Handler) registerSkillSearch(id string) {
	h.addAppPlatformTool(id, "skill_search", "For an install request call with the user's name or namespace/name and NO source unless the user explicitly named one. In order: this worker's installed files (including disabled), global library, signed-in OpenCSG, then skills.sh. An installed candidate is ready for skill_ensure_enabled; do not download it. A bare owner/repo does not identify GitHub.", platformTextFields("query", "source"), []string{"query"}, true, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
		source := platformText(args, "source")
		lookups := catalog.Lookups{Global: func(ctx context.Context, q string) ([]catalog.Candidate, error) {
			return globalSkillCandidates(ctx, q, source)
		}, Site: siteSkillCandidates, Public: catalog.SearchPublic}
		if a, ok := h.svc.Agent(id); ok && a.Role == "worker" && source != catalog.Global {
			lookups.Installed = func(ctx context.Context, q string) ([]catalog.Candidate, error) {
				if h.workspace == nil {
					return nil, errors.New("worker workspace unavailable")
				}
				layout, err := h.workspace.AgentLayout(id)
				if err != nil {
					return nil, err
				}
				var site skillSite
				if source == catalog.Site {
					site, err = loggedInSkillSite()
					if err != nil {
						return nil, nil
					}
				}
				return catalog.FindInstalled(ctx, layout.SkillsRoot, q, func(r installstate.Record) bool { return matchesLibraryOrigin(source, site.web, &r) })
			}
		}
		return catalog.Search(ctx, platformText(args, "query"), source, lookups)
	})
}

func (h *Handler) registerWorkerCatalogTools(id string) {
	fields := platformTextFields("source", "key", "selection", "site", "ref", "confirmation")
	fields["replace"] = map[string]any{"type": "boolean"}
	h.addAppPlatformTool(id, "skill_install_from_catalog", "Install one selected global or site candidate into your own worker, files only. Pass the search result's source and key, plus its exact selection for global, or site and ref for site. If the selection is stale, search again. Set replace only for an explicit reinstall. For skills.sh use the installer script.", fields, []string{"source", "key"}, false, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
		if h.agentEngine == nil {
			return nil, errors.New("agent service is unavailable")
		}
		key := platformText(args, "key")
		replace, _ := args["replace"].(bool)
		in := contract.CatalogInstall{Name: key, Replace: replace}
		source := platformText(args, "source")
		switch source {
		case catalog.Global:
			in.Selection = platformText(args, "selection")
			if platformText(args, "site") != "" || platformText(args, "ref") != "" {
				return nil, errors.New("global installs accept a library name only")
			}
		case catalog.Site:
			bundle, err := siteSkillBundle(ctx, platformText(args, "site"), key, platformText(args, "ref"))
			if err != nil {
				return nil, err
			}
			in = bundle
			in.Replace = replace
		default:
			return nil, fmt.Errorf("unsupported catalog source %q", source)
		}
		in.Confirmation = platformText(args, "confirmation")
		if _, err := h.agentEngine.Agents().Update(ctx, id, agentengine.AgentUpdateRequest{CatalogInstall: &in}); err != nil {
			var conflict *installstate.SourceConflict
			if errors.As(err, &conflict) {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: conflict.Error()}}, StructuredContent: map[string]any{"error": "skill_source_conflict", "name": conflict.Name, "installed_source": conflict.Installed, "requested_source": conflict.Requested, "confirmation": conflict.Confirmation}}, nil
			}
			return nil, err
		}
		return map[string]any{"name": in.Name, "source": source, "installed": true}, nil
	})
	h.addAppPlatformTool(id, "skill_status", "Inspect an installed skill in your current worker without enabling, repairing or reloading it. Disk sync is not session hot reload.", platformTextFields("name"), []string{"name"}, true, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
		if h.workspace == nil {
			return nil, errors.New("worker workspace is unavailable")
		}
		return h.workspace.SkillStatus(ctx, id, platformText(args, "name"))
	})
}

func siteSkillBundle(ctx context.Context, expectedSite, key, requested string) (contract.CatalogInstall, error) {
	var out contract.CatalogInstall
	site, err := loggedInSkillSite()
	if err != nil {
		return out, err
	}
	if site.api != expectedSite {
		return out, errors.New("login site changed; search again before installing")
	}
	if strings.Count(key, "/") != 1 || strings.Trim(key, "/") != key || strings.ContainsAny(key, `\?#%`) {
		return out, errors.New("expected a namespace/repository key")
	}
	if _, _, err := remote.NormalizeAgenticHubSkillRequest(key, requested); err != nil {
		return out, err
	}
	version, commit, ref, err := remote.ResolveSkillRelease(ctx, site.api, site.token, key, requested)
	if err != nil {
		return out, siteDownloadError(err)
	}
	link, err := remote.AgenticHubSkillWebURL(site.web, key)
	if err != nil {
		return out, err
	}
	archive, err := remote.FetchAgenticHubSkillArchive(ctx, site.api, site.token, key, ref)
	if err != nil {
		return out, siteDownloadError(err)
	}
	return contract.CatalogInstall{Name: path.Base(key), Archive: archive, Origin: installstate.Record{Source: installstate.SourceMarket, SourceURL: link, Ref: ref, Version: version, Commit: commit, RemotePath: key}}, nil
}

func siteDownloadError(err error) error {
	if errors.Is(err, remote.ErrAgenticHubSignInRequired) || errors.Is(err, remote.ErrRateLimited) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	return errors.New("site skill download failed; check site connectivity, repository access and selected version")
}
