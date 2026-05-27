import { useEffect, useMemo, useRef, useState } from "react";
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

const EMPTY_WORKSPACE_ENTRIES = [];

function WorkspaceFileIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <path
        d="M8.16634 1.45817C8.16634 2.58384 8.16634 3.14668 8.38433 3.5745C8.57608 3.95083 8.88204 4.25679 9.25836 4.44853C9.68618 4.66652 10.249 4.66652 11.3747 4.66652M11.6663 5.40865V9.63317C11.6663 10.7533 11.6663 11.3133 11.4484 11.7412C11.2566 12.1175 10.9506 12.4234 10.5743 12.6152C10.1465 12.8332 9.58645 12.8332 8.46634 12.8332H5.53301C4.4129 12.8332 3.85285 12.8332 3.42503 12.6152C3.0487 12.4234 2.74274 12.1175 2.55099 11.7412C2.33301 11.3133 2.33301 10.7533 2.33301 9.63317V4.36651C2.33301 3.2464 2.33301 2.68635 2.55099 2.25852C2.74274 1.8822 3.0487 1.57624 3.42503 1.38449C3.85285 1.1665 4.4129 1.1665 5.53301 1.1665H7.42419C7.91337 1.1665 8.15796 1.1665 8.38814 1.22176C8.59221 1.27076 8.7873 1.35157 8.96624 1.46122C9.16808 1.58491 9.34103 1.75786 9.68693 2.10376L10.7291 3.14591C11.075 3.49182 11.2479 3.66477 11.3716 3.8666C11.4813 4.04555 11.5621 4.24063 11.6111 4.44471C11.6663 4.67488 11.6663 4.91947 11.6663 5.40865Z"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function WorkspaceDirIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <path
        d="M3.52949 0.729004C2.5494 0.729004 2.05935 0.729004 1.68501 0.919743C1.35573 1.08752 1.08801 1.35524 0.920231 1.68452C0.729492 2.05887 0.729492 2.54891 0.729492 3.52901V9.53734C0.729492 10.8441 0.729492 11.4975 0.98381 11.9966C1.20751 12.4357 1.56447 12.7926 2.00351 13.0164C2.50264 13.2707 3.15604 13.2707 4.46283 13.2707H9.53783C10.8446 13.2707 11.498 13.2707 11.9971 13.0164C12.4362 12.7926 12.7931 12.4357 13.0168 11.9966C13.2712 11.4975 13.2712 10.8441 13.2712 9.53734V6.79567C13.2712 5.48888 13.2712 4.83549 13.0168 4.33636C12.7931 3.89731 12.4362 3.54036 11.9971 3.31666C11.498 3.06234 10.8446 3.06234 9.53783 3.06234H8.89755C8.58581 3.06234 8.42993 3.06234 8.2892 3.02677C8.05664 2.96799 7.84784 2.83894 7.69126 2.65722C7.59651 2.54725 7.5268 2.40784 7.38738 2.129C7.17826 1.71076 7.0737 1.50163 6.93157 1.33668C6.6967 1.06409 6.3835 0.870531 6.03465 0.782358C5.82356 0.729004 5.58975 0.729004 5.12213 0.729004H3.52949Z"
        fill="currentColor"
      />
    </svg>
  );
}

function HubPreviewEmptyIcon() {
  return (
    <svg
      className="hub-preview-empty-icon"
      width="32"
      height="32"
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <path
        opacity="0.12"
        d="M5.33337 13.3337V18.667C5.33337 22.4007 5.33337 24.2675 6.06 25.6936C6.69915 26.948 7.71902 27.9679 8.97344 28.607C10.3995 29.3337 12.2664 29.3337 16 29.3337C19.7337 29.3337 21.6006 29.3337 23.0266 28.607C24.2811 27.9679 25.3009 26.948 25.9401 25.6936C26.6667 24.2675 26.6667 22.4007 26.6667 18.667V12.8003C26.6667 12.0536 26.6667 11.6802 26.5214 11.395C26.3936 11.1441 26.1896 10.9401 25.9387 10.8123C25.6535 10.667 25.2801 10.667 24.5334 10.667H22.9334C21.4399 10.667 20.6932 10.667 20.1227 10.3763C19.621 10.1207 19.213 9.71273 18.9574 9.21097C18.6667 8.64054 18.6667 7.8938 18.6667 6.40033V4.80033C18.6667 4.05359 18.6667 3.68022 18.5214 3.395C18.3936 3.14412 18.1896 2.94015 17.9387 2.81232C17.6535 2.66699 17.2801 2.66699 16.5334 2.66699H16C12.2664 2.66699 10.3995 2.66699 8.97344 3.39362C7.71902 4.03277 6.69915 5.05264 6.06 6.30706C5.33337 7.73313 5.33337 9.59997 5.33337 13.3337Z"
        fill="#4D6AD6"
      />
      <path
        d="M18.6667 3.33366V6.40037C18.6667 7.89384 18.6667 8.64058 18.9574 9.21101C19.213 9.71277 19.621 10.1207 20.1227 10.3764C20.6932 10.667 21.4399 10.667 22.9334 10.667H26M12 16.0003H20M12 21.3337H17.3334M26.6667 11.9846V22.9337C26.6667 25.1739 26.6667 26.294 26.2307 27.1496C25.8472 27.9023 25.2353 28.5142 24.4827 28.8977C23.627 29.3337 22.5069 29.3337 20.2667 29.3337H11.7334C9.49317 29.3337 8.37306 29.3337 7.51741 28.8977C6.76476 28.5142 6.15284 27.9023 5.76935 27.1496C5.33337 26.294 5.33337 25.1739 5.33337 22.9337V9.06699C5.33337 6.82678 5.33337 5.70668 5.76935 4.85103C6.15284 4.09838 6.76476 3.48646 7.51741 3.10297C8.37306 2.66699 9.49316 2.66699 11.7334 2.66699H17.3491C18.3274 2.66699 18.8166 2.66699 19.277 2.77751C19.6851 2.8755 20.0753 3.03712 20.4332 3.25643C20.8368 3.5038 21.1828 3.8497 21.8746 4.54151L24.7922 7.45914C25.484 8.15095 25.8299 8.49685 26.0773 8.90052C26.2966 9.25841 26.4582 9.64859 26.5562 10.0567C26.6667 10.5171 26.6667 11.0063 26.6667 11.9846Z"
        stroke="#4D6AD6"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

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
  const workspaceEntries = selectedTemplate?.workspace?.entries ?? EMPTY_WORKSPACE_ENTRIES;
  const [collapsedWorkspaceDirs, setCollapsedWorkspaceDirs] = useState({});
  const [isTemplateListScrolling, setIsTemplateListScrolling] = useState(false);
  const [isInspectorScrolling, setIsInspectorScrolling] = useState(false);
  const templateListScrollTimerRef = useRef<number | null>(null);
  const inspectorScrollTimerRef = useRef<number | null>(null);
  const visibleWorkspaceEntries = useMemo(
    () => buildVisibleHubWorkspaceEntries(workspaceEntries, collapsedWorkspaceDirs),
    [workspaceEntries, collapsedWorkspaceDirs],
  );
  const workspacePreviewText =
    workspaceFile && !workspaceFile.binary ? workspaceFile.content || t("hubWorkspaceEmptyFile") : "";
  const workspacePreviewLineCount = workspacePreviewText ? workspacePreviewText.split(/\r\n|\r|\n/).length : 0;

  useEffect(
    () => () => {
      if (templateListScrollTimerRef.current) {
        window.clearTimeout(templateListScrollTimerRef.current);
      }
      if (inspectorScrollTimerRef.current) {
        window.clearTimeout(inspectorScrollTimerRef.current);
      }
    },
    [],
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

  function handleTemplateListScroll() {
    setIsTemplateListScrolling(true);
    if (templateListScrollTimerRef.current) {
      window.clearTimeout(templateListScrollTimerRef.current);
    }
    templateListScrollTimerRef.current = window.setTimeout(() => {
      setIsTemplateListScrolling(false);
      templateListScrollTimerRef.current = null;
    }, 900);
  }

  function handleInspectorScroll() {
    setIsInspectorScrolling(true);
    if (inspectorScrollTimerRef.current) {
      window.clearTimeout(inspectorScrollTimerRef.current);
    }
    inspectorScrollTimerRef.current = window.setTimeout(() => {
      setIsInspectorScrolling(false);
      inspectorScrollTimerRef.current = null;
    }, 900);
  }

  return (
    <section className="entity-pane hub-detail-pane">
      <header className="hub-page-header">
        <div className="hub-page-heading">
          <h1>{t("hubTitle")}</h1>
          <p>{t("hubSubtitle")}</p>
        </div>
        <Button variant="secondaryGray" size="md" onClick={onRetry}>
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
            <div
              className={`hub-template-list ${isTemplateListScrolling ? "is-scrolling" : ""}`}
              onScroll={handleTemplateListScroll}
            >
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
          </div>

          <div
            className={`hub-inspector-panel ${isInspectorScrolling ? "is-scrolling" : ""}`}
            onScroll={handleInspectorScroll}
          >
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
                        <div className="hub-inspector-badge-row">
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
                    </div>
                    <div className="hub-template-actions">
                      <Button variant="primary" size="md" onClick={() => onCreateFromTemplate?.(selectedTemplate)}>
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
                              <span className="hub-tree-glyph" aria-hidden="true">
                                {entry.type === "dir" ? <WorkspaceDirIcon /> : <WorkspaceFileIcon />}
                              </span>
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
                          <HubPreviewEmptyIcon />
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
                              <div className="hub-preview-code-shell">
                                <pre className="hub-preview-line-numbers">
                                  {Array.from({ length: workspacePreviewLineCount }, (_, index) => index + 1).join(
                                    "\n",
                                  )}
                                </pre>
                                <pre className="hub-preview-code">{workspacePreviewText}</pre>
                              </div>
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
