import { useEffect, useRef, useState } from "react";
import {
  formatHubDate,
  formatHubDateTime,
  formatHubTemplateCount,
  isDeletableHubTemplate,
} from "@/models/hubWorkspace";
import { WorkspaceFilePreview, WorkspaceFileTree } from "@/components/business/WorkspaceFileTree";
import { localizeRole, localizeTemplateSourceTag } from "@/shared/i18n";
import { HubIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { WorkspaceEntry, WorkspaceFile } from "@/models/workspace";

const EMPTY_WORKSPACE_ENTRIES: readonly WorkspaceEntry[] = [];

type HubDetailPaneHub = {
  detailPaneProps: {
    deleteBusy?: boolean;
    detailLoading: boolean;
    error: string;
    loaded: boolean;
    onDeleteTemplate?: (item: HubTemplate | null | undefined) => unknown;
    onRetry: () => void | Promise<void>;
    onSelectTemplate?: (item: HubTemplate | null | undefined) => void;
    onSelectWorkspaceFile: (workspacePath: string) => void;
    selectedTemplate: HubTemplate | null;
    selectedTemplateId: string;
    selectedWorkspacePath: string;
    templates: readonly HubTemplate[];
    workspaceFile: WorkspaceFile | null;
    workspaceFileError: string;
    workspaceFileLoading: boolean;
  };
};

const EMPTY_HUB_DETAIL_PROPS: HubDetailPaneHub["detailPaneProps"] = {
  deleteBusy: false,
  detailLoading: false,
  error: "",
  loaded: false,
  onRetry: () => {},
  onSelectWorkspaceFile: () => {},
  selectedTemplate: null,
  selectedTemplateId: "",
  selectedWorkspacePath: "",
  templates: [],
  workspaceFile: null,
  workspaceFileError: "",
  workspaceFileLoading: false,
};

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

export type HubDetailPaneProps = {
  hub?: HubDetailPaneHub;
  locale?: LocaleCode;
  onCreateFromTemplate?: (item: HubTemplate) => void | Promise<void>;
  t?: TranslateFn;
};

export function HubDetailPane({
  t = (key) => key,
  locale = "en",
  hub,
  onCreateFromTemplate = () => {},
}: HubDetailPaneProps) {
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
    onDeleteTemplate,
    deleteBusy = false,
  } = hub?.detailPaneProps ?? EMPTY_HUB_DETAIL_PROPS;
  const canDeleteTemplate = isDeletableHubTemplate(selectedTemplate);
  const workspaceEntries = selectedTemplate?.workspace?.entries ?? EMPTY_WORKSPACE_ENTRIES;
  const [isTemplateListScrolling, setIsTemplateListScrolling] = useState(false);
  const [isInspectorScrolling, setIsInspectorScrolling] = useState(false);
  const templateListScrollTimerRef = useRef<number | null>(null);
  const inspectorScrollTimerRef = useRef<number | null>(null);
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
                      {canDeleteTemplate ? (
                        <Button
                          variant="danger"
                          size="md"
                          loading={deleteBusy}
                          disabled={deleteBusy}
                          onClick={() => onDeleteTemplate?.(selectedTemplate)}
                        >
                          {t("hubDeleteTemplate")}
                        </Button>
                      ) : null}
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
                    <WorkspaceFileTree
                      className="hub-workspace-tree"
                      entries={workspaceEntries}
                      loading={detailLoading}
                      loadingText={t("hubWorkspaceLoading")}
                      emptyText={t("hubWorkspacePreviewHint")}
                      selectedPath={selectedWorkspacePath}
                      onSelectFile={onSelectWorkspaceFile}
                    />
                    <WorkspaceFilePreview
                      className="hub-workspace-preview"
                      file={workspaceFile}
                      loading={workspaceFileLoading}
                      error={workspaceFileError}
                      loadingText={t("hubWorkspaceFileLoading")}
                      emptyTitle={t("hubWorkspacePreviewTitle")}
                      emptyHint={t("hubWorkspacePreviewHint")}
                      emptyIcon={<HubPreviewEmptyIcon />}
                      binaryText={t("hubWorkspaceBinary")}
                      emptyFileText={t("hubWorkspaceEmptyFile")}
                      previewText={t("workspacePreviewPreviewTab")}
                      codeText={t("workspacePreviewCodeTab")}
                      viewToggleLabel={t("workspacePreviewViewMode")}
                      closeText={t("close")}
                      truncatedText={t("workspacePreviewTruncated")}
                    />
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
