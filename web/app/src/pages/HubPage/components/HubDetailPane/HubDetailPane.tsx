import { useEffect, useMemo, useState } from "react";
import {
  buildInitialCollapsedHubWorkspaceDirs,
  buildVisibleHubWorkspaceEntries,
  formatHubDate,
  formatHubDateTime,
  formatHubTemplateCount,
  hubWorkspaceAncestorDirs,
} from "@/models/hubWorkspace";
import type { HubTreeDepthStyle } from "@/models/hubWorkspace";
import { localizeRole, localizeTemplateSourceTag } from "@/shared/i18n";
import { HubIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";

export function HubDetailPane({ t, locale, hub, onCreateFromTemplate }) {
  const {
    templates,
    selectedTemplate,
    selectedTemplateId,
    loaded,
    error,
    detailLoading,
    selectedWorkspacePath,
    workspaceFile,
    workspaceFileLoading,
    workspaceFileError,
    onRetry,
    onSelectTemplate,
    onSelectWorkspaceFile,
  } = hub.detailPaneProps;
  const workspaceEntries = selectedTemplate?.workspace?.entries || [];
  const [collapsedWorkspaceDirs, setCollapsedWorkspaceDirs] = useState({});
  const visibleWorkspaceEntries = useMemo(
    () => buildVisibleHubWorkspaceEntries(workspaceEntries, collapsedWorkspaceDirs),
    [workspaceEntries, collapsedWorkspaceDirs],
  );

  useEffect(() => {
    setCollapsedWorkspaceDirs(buildInitialCollapsedHubWorkspaceDirs(workspaceEntries));
  }, [selectedTemplate?.id, workspaceEntries]);

  useEffect(() => {
    if (!selectedWorkspacePath) {
      return;
    }
    const ancestors = hubWorkspaceAncestorDirs(selectedWorkspacePath);
    if (!ancestors.length) {
      return;
    }
    setCollapsedWorkspaceDirs((current) => {
      let changed = false;
      const next = { ...current };
      ancestors.forEach((path) => {
        if (next[path]) {
          delete next[path];
          changed = true;
        }
      });
      return changed ? next : current;
    });
  }, [selectedWorkspacePath]);

  function toggleWorkspaceDir(path) {
    setCollapsedWorkspaceDirs((current) => ({
      ...current,
      [path]: !current[path],
    }));
  }

  return (
    <section className="entity-pane hub-detail-pane">
      <header className="hub-page-header">
        <div className="hub-page-heading">
          <h1>{t("hubTitle")}</h1>
          <p>{t("hubSubtitle")}</p>
        </div>
        <Button className="preview-action-button" onClick={onRetry}>
          {loaded ? t("hubRefresh") : t("hubLoading")}
        </Button>
      </header>
      {error ? <div className="form-error">{error}</div> : null}
      {!loaded && !error ? (
        <div className="workspace-empty">{t("hubLoading")}</div>
      ) : templates.length === 0 ? (
        <div className="empty-state shell-empty-state hub-empty-state">
          <span className="rich-empty-mark" aria-hidden="true">
            *
          </span>
          <strong>{t("hubEmpty")}</strong>
        </div>
      ) : (
        <div className="hub-workbench">
          <div className="hub-catalog-panel">
            <div className="hub-filter-tabs">
              <button type="button" className="hub-filter-tab active">
                {t("hubAllTab")}
              </button>
            </div>
            <div className="hub-catalog-meta">{formatHubTemplateCount(templates.length, locale, t)}</div>
            <div className="hub-template-list">
              {templates.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  className={`hub-template-card ${selectedTemplateId === item.id ? "active" : ""}`}
                  onClick={() => onSelectTemplate?.(item)}
                >
                  <div className="hub-template-card-icon">
                    <HubIcon />
                  </div>
                  <div className="hub-template-card-body">
                    <div className="hub-template-card-title-row">
                      <h2>{item.name || item.id}</h2>
                    </div>
                    <p>{item.description || item.id}</p>
                    <div className="hub-template-card-meta">
                      <span className="mini-badge template-role-badge">{localizeRole(item.role || "worker", t)}</span>
                      <span className="mini-badge template-runtime-badge">
                        {item.runtime_kind || item.workspace?.kind || "-"}
                      </span>
                      <span className="mini-badge template-source-badge">
                        <span className="template-source-badge-dot" aria-hidden="true"></span>
                        {localizeTemplateSourceTag(item.source?.name, locale)}
                      </span>
                      <span className="hub-template-card-updated">
                        {t("hubUpdatedAtLabel")} {formatHubDate(item.updated_at, locale)}
                      </span>
                    </div>
                  </div>
                </button>
              ))}
            </div>
            <div className="hub-catalog-end">{t("hubListEnd")}</div>
          </div>

          <div className="hub-inspector-panel">
            {selectedTemplate ? (
              <>
                <div className="hub-inspector-hero">
                  <div className="hub-inspector-hero-row">
                    <div className="hub-inspector-brand">
                      <div className="hub-inspector-icon">
                        <HubIcon />
                      </div>
                      <div className="hub-inspector-copy">
                        <h2>{selectedTemplate.name || selectedTemplate.id}</h2>
                        <p>{selectedTemplate.description || selectedTemplate.id}</p>
                        <span className="mini-badge template-role-badge">
                          {localizeRole(selectedTemplate.role || "worker", t)}
                        </span>
                        <span className="mini-badge template-runtime-badge">
                          {selectedTemplate.runtime_kind || selectedTemplate.workspace?.kind || "-"}
                        </span>
                        <span className="mini-badge template-source-badge">
                          <span className="template-source-badge-dot" aria-hidden="true"></span>
                          {localizeTemplateSourceTag(selectedTemplate.source?.name, locale)}
                        </span>
                      </div>
                    </div>
                    <div className="hub-template-actions">
                      <Button
                        variant="primary"
                        className="preview-action-button preview-action-button-primary"
                        onClick={() => onCreateFromTemplate?.(selectedTemplate)}
                      >
                        <span>{t("createAgent")}</span>
                      </Button>
                    </div>
                  </div>
                </div>

                <div className="hub-inspector-grid">
                  <div className="hub-inspector-field">
                    <span>{t("roleLabel")}</span>
                    <strong>{localizeRole(selectedTemplate.role || "worker", t)}</strong>
                  </div>
                  <div className="hub-inspector-field">
                    <span>{t("hubSourceLabel")}</span>
                    <strong>{localizeTemplateSourceTag(selectedTemplate.source?.name, locale)}</strong>
                  </div>
                  <div className="hub-inspector-field">
                    <span>{t("hubRuntimeLabel")}</span>
                    <strong>{selectedTemplate.runtime_kind || "-"}</strong>
                  </div>
                  <div className="hub-inspector-field">
                    <span>{t("hubImageLabel")}</span>
                    <strong className="hub-field-value-multiline">{selectedTemplate.image || "-"}</strong>
                  </div>
                  <div className="hub-inspector-field">
                    <span>{t("hubUpdatedAtLabel")}</span>
                    <strong>{formatHubDateTime(selectedTemplate.updated_at, locale)}</strong>
                  </div>
                </div>

                <div className="hub-description-block">
                  <span className="hub-section-label">{t("hubDescriptionLabel")}</span>
                  <p>{selectedTemplate.description || selectedTemplate.id}</p>
                </div>

                <div className="hub-workspace-block">
                  <span className="hub-section-label">{t("hubWorkspaceTemplateLabel")}</span>
                  <div className="hub-workspace-panels">
                    <div className="hub-workspace-tree">
                      {detailLoading ? (
                        <div className="workspace-empty">{t("hubWorkspaceLoading")}</div>
                      ) : workspaceEntries.length === 0 ? (
                        <div className="workspace-empty">{t("hubWorkspacePreviewHint")}</div>
                      ) : (
                        visibleWorkspaceEntries.map((entry) => {
                          const collapsed = entry.type === "dir" && Boolean(collapsedWorkspaceDirs[entry.path]);
                          return (
                            <button
                              key={entry.path}
                              type="button"
                              className={`hub-tree-row ${entry.type} ${entry.type === "dir" ? "toggleable" : ""} ${entry.type === "file" && selectedWorkspacePath === entry.path ? "active" : ""}`.trim()}
                              style={{ "--hub-tree-depth": entry.depth ?? 0 } as HubTreeDepthStyle}
                              onClick={() =>
                                entry.type === "dir"
                                  ? toggleWorkspaceDir(entry.path)
                                  : onSelectWorkspaceFile?.(entry.path)
                              }
                              aria-expanded={entry.type === "dir" ? !collapsed : undefined}
                            >
                              <span
                                className={`hub-tree-toggle ${entry.type === "file" ? "spacer" : ""} ${collapsed ? "collapsed" : ""}`.trim()}
                                aria-hidden="true"
                              ></span>
                              <span className="hub-tree-glyph" aria-hidden="true"></span>
                              <span className="hub-tree-label">{entry.name}</span>
                            </button>
                          );
                        })
                      )}
                    </div>
                    <div className="hub-workspace-preview">
                      {workspaceFileError ? (
                        <div className="workspace-empty">{workspaceFileError}</div>
                      ) : workspaceFileLoading ? (
                        <div className="workspace-empty">{t("hubWorkspaceFileLoading")}</div>
                      ) : !workspaceFile ? (
                        <div className="hub-preview-empty-state">
                          <div className="hub-preview-empty-icon" aria-hidden="true"></div>
                          <strong>{t("hubWorkspacePreviewTitle")}</strong>
                          <p>{t("hubWorkspacePreviewHint")}</p>
                        </div>
                      ) : (
                        <>
                          <div className="hub-preview-file-header">
                            <strong>{workspaceFile.path}</strong>
                            <span>
                              {workspaceFile.binary ? t("hubWorkspaceBinary") : `${workspaceFile.size || 0} B`}
                            </span>
                          </div>
                          <div className="hub-preview-body">
                            {workspaceFile.binary ? (
                              <div className="workspace-empty">{t("hubWorkspaceBinary")}</div>
                            ) : (
                              <pre className="hub-preview-code">
                                {workspaceFile.content || t("hubWorkspaceEmptyFile")}
                              </pre>
                            )}
                          </div>
                        </>
                      )}
                    </div>
                  </div>
                </div>
              </>
            ) : null}
          </div>
        </div>
      )}
    </section>
  );
}
