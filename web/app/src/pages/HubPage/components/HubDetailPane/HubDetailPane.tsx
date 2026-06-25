import { useEffect, useMemo, useRef, useState } from "react";
import { FileCode2 } from "lucide-react";
import { formatHubDateTime, isDeletableHubTemplate } from "@/models/hubWorkspace";
import { WorkspaceFilePreview, WorkspaceFileTree } from "@/components/business/WorkspaceFileTree";
import { localizeTemplateSourceTag } from "@/shared/i18n";
import { HubIcon } from "@/components/ui/Icons";
import {
  Button,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { SkillFile, SkillSummary, SkillTree } from "@/models/skillhub";
import type { WorkspaceEntry, WorkspaceFile } from "@/models/workspace";

const EMPTY_WORKSPACE_ENTRIES: readonly WorkspaceEntry[] = [];

type HubDetailPaneHub = {
  detailPaneProps: {
    deleteBusy?: boolean;
    detailLoading: boolean;
    error: string;
    loaded: boolean;
    onDeleteSkill?: (item: SkillSummary | null | undefined) => Promise<boolean> | boolean;
    onDeleteTemplate?: (item: HubTemplate | null | undefined) => unknown;
    onRetry: () => void | Promise<void>;
    onSelectSkill?: (name: string | null | undefined) => void;
    onSelectSkillFile?: (path: string) => void;
    onSelectTemplate?: (item: HubTemplate | null | undefined) => void;
    onSelectWorkspaceFile: (workspacePath: string) => void;
    selectedResourceType?: "skill" | "template";
    selectedSkill: SkillSummary | null;
    selectedSkillPath: string;
    selectedTemplate: HubTemplate | null;
    selectedTemplateId: string;
    selectedWorkspacePath: string;
    skillFile: SkillFile | null;
    skillFileError: string;
    skillFileLoading: boolean;
    skillDeleteBusy?: boolean;
    skills: readonly SkillSummary[];
    skillTree: SkillTree | null;
    skillTreeError: string;
    skillTreeLoading: boolean;
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
  onSelectSkillFile: () => {},
  onSelectWorkspaceFile: () => {},
  selectedResourceType: "template",
  selectedSkill: null,
  selectedSkillPath: "",
  selectedTemplate: null,
  selectedTemplateId: "",
  selectedWorkspacePath: "",
  skillFile: null,
  skillFileError: "",
  skillFileLoading: false,
  skillDeleteBusy: false,
  skills: [],
  skillTree: null,
  skillTreeError: "",
  skillTreeLoading: false,
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
    skills,
    selectedTemplate,
    selectedSkill,
    selectedSkillPath,
    selectedResourceType = "template",
    loaded,
    error,
    detailLoading,
    selectedWorkspacePath,
    workspaceFile,
    workspaceFileLoading,
    workspaceFileError,
    skillTree,
    skillTreeLoading,
    skillTreeError,
    skillFile,
    skillFileLoading,
    skillFileError,
    onSelectWorkspaceFile,
    onSelectSkillFile,
    onDeleteSkill,
    onDeleteTemplate,
    deleteBusy = false,
    skillDeleteBusy = false,
  } = hub?.detailPaneProps ?? EMPTY_HUB_DETAIL_PROPS;
  const canDeleteTemplate = isDeletableHubTemplate(selectedTemplate);
  const workspaceEntries = selectedTemplate?.workspace?.entries ?? EMPTY_WORKSPACE_ENTRIES;
  const skillEntries = skillTree?.entries ?? EMPTY_WORKSPACE_ENTRIES;
  const activeResourceType = useMemo(() => {
    if (selectedResourceType === "skill" && skills.length) {
      return "skill";
    }
    if (templates.length) {
      return "template";
    }
    if (skills.length) {
      return "skill";
    }
    return "template";
  }, [selectedResourceType, skills.length, templates.length]);
  const [isInspectorScrolling, setIsInspectorScrolling] = useState(false);
  const [deleteSkillDialogOpen, setDeleteSkillDialogOpen] = useState(false);
  const inspectorScrollTimerRef = useRef<number | null>(null);
  useEffect(
    () => () => {
      if (inspectorScrollTimerRef.current) {
        window.clearTimeout(inspectorScrollTimerRef.current);
      }
    },
    [],
  );

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

  async function handleDeleteSkillConfirm() {
    const deleted = await onDeleteSkill?.(selectedSkill);
    if (deleted) {
      setDeleteSkillDialogOpen(false);
    }
  }

  return (
    <section className="entity-pane hub-detail-pane">
      {error ? <div className="form-error">{error}</div> : null}
      {!loaded && !error ? (
        <div className="workspace-empty">{t("hubLoading")}</div>
      ) : templates.length === 0 && skills.length === 0 ? (
        <div className="empty-state shell-empty-state hub-empty-state">
          <span className="rich-empty-mark" aria-hidden="true">
            *
          </span>
          <strong>{t("hubEmpty")}</strong>
        </div>
      ) : (
        <div
          className={`hub-workbench hub-inspector-panel ${isInspectorScrolling ? "is-scrolling" : ""}`}
          onScroll={handleInspectorScroll}
        >
          {activeResourceType === "template" && selectedTemplate ? (
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
          ) : activeResourceType === "skill" && selectedSkill ? (
            <>
              <div className="hub-inspector-hero">
                <div className="hub-inspector-hero-row">
                  <div className="hub-inspector-brand">
                    <div className="hub-inspector-icon hub-skill-card-icon">
                      <FileCode2 aria-hidden="true" />
                    </div>
                    <div className="hub-inspector-copy">
                      <h2>{selectedSkill.name}</h2>
                      <p>{selectedSkill.description || selectedSkill.name}</p>
                    </div>
                  </div>
                  <div className="hub-template-actions">
                    <Button
                      className="hub-skill-delete-button"
                      variant="outlineDanger"
                      size="md"
                      disabled={skillDeleteBusy}
                      onClick={() => setDeleteSkillDialogOpen(true)}
                    >
                      {t("hubDeleteSkill")}
                    </Button>
                  </div>
                </div>
              </div>

              <div className="hub-workspace-block">
                <div className="hub-workspace-panels">
                  <WorkspaceFileTree
                    className="hub-workspace-tree"
                    entries={skillEntries}
                    loading={skillTreeLoading}
                    loadingText={t("hubSkillFilesLoading")}
                    emptyText={skillTreeError || t("hubSkillFilesEmpty")}
                    selectedPath={selectedSkillPath}
                    onSelectFile={onSelectSkillFile}
                  />
                  <WorkspaceFilePreview
                    className="hub-workspace-preview"
                    file={skillFile}
                    loading={skillFileLoading}
                    error={skillFileError}
                    loadingText={t("hubWorkspaceFileLoading")}
                    emptyTitle={t("hubSkillPreviewTitle")}
                    emptyHint={t("hubSkillPreviewHint")}
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
          ) : (
            <div className="empty-state shell-empty-state hub-empty-state">
              <span className="rich-empty-mark" aria-hidden="true">
                *
              </span>
              <strong>{templates.length || skills.length ? t("hubLoading") : t("hubEmpty")}</strong>
            </div>
          )}
        </div>
      )}
      <DialogRoot open={deleteSkillDialogOpen} onOpenChange={setDeleteSkillDialogOpen}>
        <DialogContent className="hub-skill-delete-dialog">
          <DialogHeader className="hub-skill-delete-dialog-header">
            <div className="hub-skill-delete-dialog-copy">
              <DialogTitle>{t("hubDeleteSkill")}</DialogTitle>
              <DialogDescription>
                {t("hubDeleteSkillConfirmMessage", { name: selectedSkill?.name || "" })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <DialogFooter className="hub-skill-delete-dialog-actions">
            <Button
              variant="secondaryGray"
              size="sm"
              disabled={skillDeleteBusy}
              onClick={() => setDeleteSkillDialogOpen(false)}
            >
              {t("cancel")}
            </Button>
            <Button variant="danger" size="sm" loading={skillDeleteBusy} onClick={handleDeleteSkillConfirm}>
              {t("hubDeleteSkillConfirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </section>
  );
}
