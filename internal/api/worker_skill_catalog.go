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
			c.Version = rec.Version
			c.Ref = rec.Ref
			c.URL = rec.DisplayURL()
		}
		switch source {
		case catalog.Site:
			if item.Install == nil || item.Install.SourceKind() != installstate.SourceMarket {
				continue
			}
			expected, err := remote.AgenticHubSkillWebURL(current.web, item.Install.RemotePath)
			if err != nil || expected != c.URL {
				continue
			}
		case catalog.Public:
			if !catalog.ValidPublicURL(c.URL) {
				continue
			}
		}
		out = append(out, c)
	}
	return catalog.Match(out, query), ctx.Err()
}

func siteSkillCandidates(ctx context.Context, query string) ([]catalog.Candidate, error) {
	site, err := loggedInSkillSite()
	if err != nil {
		return nil, err
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
	h.addAppPlatformTool(id, "skill_search", "Search installable skills. By default a global library match avoids all network calls; otherwise search the signed-in CSGHub site and skills.sh concurrently. Specify source only when the user restricts the search. Report per-source failures, especially login_required.", platformTextFields("query", "source"), []string{"query"}, true, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
		source := platformText(args, "source")
		return catalog.Search(ctx, platformText(args, "query"), source, func(ctx context.Context, q string) ([]catalog.Candidate, error) {
			return globalSkillCandidates(ctx, q, source)
		}, siteSkillCandidates, catalog.SearchPublic)
	})
}

func (h *Handler) registerWorkerCatalogTools(id string) {
	fields := platformTextFields("source", "key", "site", "ref")
	fields["replace"] = map[string]any{"type": "boolean"}
	h.addAppPlatformTool(id, "skill_install_from_catalog", "Install one selected global or site candidate into your own worker, files only. Pass the search result's source and key (also site and ref for site). Set replace only for an explicit reinstall. For skills.sh use the installer script.", fields, []string{"source", "key"}, false, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (any, error) {
		if h.agentEngine == nil {
			return nil, errors.New("agent service is unavailable")
		}
		key := platformText(args, "key")
		replace, _ := args["replace"].(bool)
		in := contract.CatalogInstall{Name: key, Replace: replace}
		source := platformText(args, "source")
		switch source {
		case catalog.Global:
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
		if _, err := h.agentEngine.Agents().Update(ctx, id, agentengine.AgentUpdateRequest{CatalogInstall: &in}); err != nil {
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
