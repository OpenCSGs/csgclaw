import { useEffect, useId, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { json } from "@codemirror/lang-json";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { linter } from "@codemirror/lint";
import type { Diagnostic } from "@codemirror/lint";
import { EditorState, type Extension } from "@codemirror/state";
import { EditorView, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import { FileCode2, Server, Trash2 } from "lucide-react";
import { formatRuntimeKindLabel } from "@/models/agents";
import { formatHubDateTime, isDeletableHubTemplate } from "@/models/hubWorkspace";
import { formatMCPServerDocument, mcpServerDescription, mcpServerPayloadFromDocument } from "@/models/mcp";
import type { MCPServerPayload } from "@/models/mcp";
import { WorkspaceFilePreview, WorkspaceFileTree } from "@/components/business/WorkspaceFileTree";
import { localizeTemplateSourceTag } from "@/shared/i18n";
import { ModelsIcon } from "@/components/ui/Icons";
import {
  Button,
  DialogBody,
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
import type { MCPServer } from "@/models/mcp";
import { isReadonlySkill } from "@/models/skillhub";
import type { SkillFile, SkillSummary, SkillTree } from "@/models/skillhub";
import type { WorkspaceEntry, WorkspaceFile } from "@/models/workspace";

const EMPTY_WORKSPACE_ENTRIES: readonly WorkspaceEntry[] = [];

type HubDetailPaneHub = {
  detailPaneProps: {
    deleteBusy?: boolean;
    detailLoading?: boolean;
    error: string;
    loaded: boolean;
    onDeleteSkill?: (item: SkillSummary | null | undefined) => Promise<boolean> | boolean;
    onCreateMCP?: (payload: MCPServerPayload) => Promise<boolean> | boolean;
    onDeleteMCP?: (item: MCPServer | null | undefined) => Promise<boolean> | boolean;
    onDeleteTemplate?: (item: HubTemplate | null | undefined) => unknown;
    onSelectMCP?: (name: string | null | undefined) => void;
    onUpdateMCP?: (currentName: string, payload: MCPServerPayload) => Promise<boolean> | boolean;
    onRetry: () => void | Promise<void>;
    onSelectSkill?: (name: string | null | undefined) => void;
    onSelectSkillFile?: (path: string) => void;
    onSelectTemplate?: (item: HubTemplate | null | undefined) => void;
    onSelectWorkspaceFile: (workspacePath: string) => void;
    onToggleWorkspaceDir?: (workspacePath: string) => void | Promise<void>;
    mcpServers?: readonly MCPServer[];
    mcpStateError?: string;
    mcpStateLoading?: boolean;
    mcpMutationBusy?: boolean;
    mcpMutationError?: string;
    mcpCreateDialogOpen?: boolean;
    onMCPCreateDialogOpenChange?: (open: boolean) => void;
    selectedMCPServer?: MCPServer | null;
    selectedMCPServerName?: string;
    selectedResourceType?: "mcp" | "skill" | "template";
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
    workspaceEntries?: readonly WorkspaceEntry[];
    workspaceTreeLoading?: boolean;
    loadingWorkspaceDirs?: ReadonlySet<string>;
  };
};

const EMPTY_HUB_DETAIL_PROPS: HubDetailPaneHub["detailPaneProps"] = {
  deleteBusy: false,
  error: "",
  loaded: false,
  onRetry: () => {},
  mcpServers: [],
  mcpStateError: "",
  mcpStateLoading: false,
  mcpMutationBusy: false,
  mcpMutationError: "",
  mcpCreateDialogOpen: false,
  selectedMCPServer: null,
  selectedMCPServerName: "",
  onSelectSkillFile: () => {},
  onSelectWorkspaceFile: () => {},
  onToggleWorkspaceDir: () => {},
  onCreateMCP: () => false,
  onDeleteMCP: () => false,
  onUpdateMCP: () => false,
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
  workspaceEntries: [],
  workspaceTreeLoading: false,
  loadingWorkspaceDirs: new Set(),
};

const DEFAULT_MCP_SERVER_DOCUMENT =
  '{\n  "mcpServers": {\n    "filesystem": {\n      "command": "npx",\n      "args": ["-y", "@modelcontextprotocol/server-filesystem", "${workspace}"],\n      "startup_timeout_sec": 60\n    }\n  }\n}';

const jsonEditorTheme = EditorView.theme({
  "&": {
    backgroundColor: "transparent",
    color: "var(--text)",
    fontFamily: "var(--font-mono)",
    fontSize: "12px",
  },
  "&.cm-focused": {
    outline: "none",
  },
  ".cm-scroller": {
    fontFamily: "var(--font-mono)",
    lineHeight: "1.6",
    minHeight: "var(--hub-json-editor-min-height, 220px)",
  },
  ".cm-content": {
    caretColor: "var(--text)",
    padding: "14px 0",
  },
  ".cm-line": {
    padding: "0 14px",
  },
  ".cm-gutters": {
    backgroundColor: "transparent",
    borderRight: "1px solid color-mix(in oklab, var(--line) 70%, transparent)",
    color: "var(--gray-500)",
    paddingLeft: "4px",
  },
  ".cm-activeLine": {
    backgroundColor: "color-mix(in oklab, var(--brand-600) 7%, transparent)",
  },
  ".cm-activeLineGutter": {
    backgroundColor: "color-mix(in oklab, var(--brand-600) 7%, transparent)",
    color: "var(--gray-700)",
  },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
    backgroundColor: "color-mix(in oklab, var(--brand-600) 24%, transparent)",
  },
  ".cm-lintRange-error": {
    textDecoration: "underline wavy var(--error-600)",
    textDecorationSkipInk: "none",
  },
  ".cm-tooltip": {
    border: "1px solid var(--line)",
    borderRadius: "var(--radius-md)",
    backgroundColor: "var(--surface)",
    color: "var(--text)",
    boxShadow: "var(--shadow-lg)",
    fontFamily: "var(--font-sans)",
    fontSize: "12px",
  },
});

const jsonHighlightStyle = HighlightStyle.define([
  { tag: tags.propertyName, color: "var(--brand-700)" },
  { tag: tags.string, color: "var(--success-700)" },
  { tag: tags.number, color: "var(--warning-700)" },
  { tag: tags.bool, color: "var(--error-600)" },
  { tag: tags.null, color: "var(--error-600)" },
  { tag: tags.punctuation, color: "var(--gray-500)" },
]);

function jsonSyntaxLinter(view: EditorView): Diagnostic[] {
  const source = view.state.doc.toString();
  try {
    JSON.parse(source);
    return [];
  } catch (error) {
    const message = error instanceof Error ? error.message : "Invalid JSON";
    const positionMatch = /position\s+(\d+)/i.exec(message);
    const parsedPosition = positionMatch ? Number(positionMatch[1]) : source.length;
    if (source.length === 0) {
      return [
        {
          from: 0,
          message,
          severity: "error",
          to: 0,
        },
      ];
    }
    const position = Number.isFinite(parsedPosition) ? Math.max(0, Math.min(source.length, parsedPosition)) : 0;
    const from = Math.max(0, Math.min(source.length, position || source.length) - 1);
    const to = Math.min(source.length, Math.max(from + 1, position + 1));
    return [
      {
        from,
        message,
        severity: "error",
        to,
      },
    ];
  }
}

const jsonEditorExtensions: Extension[] = [
  lineNumbers(),
  highlightActiveLineGutter(),
  highlightActiveLine(),
  history(),
  json(),
  syntaxHighlighting(jsonHighlightStyle),
  linter(jsonSyntaxLinter, { delay: 250 }),
  keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap]),
  EditorState.tabSize.of(2),
  EditorView.lineWrapping,
  jsonEditorTheme,
];

type MCPServerDocumentParseResult =
  | {
      kind: "valid";
      payload: MCPServerPayload;
    }
  | {
      kind: "structure" | "syntax";
      message: string;
    };

function parseMCPServerDocument(value: string, t: TranslateFn): MCPServerDocumentParseResult {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    return { kind: "syntax", message: t("resourcesMCPServerDocumentInvalid") };
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    return { kind: "structure", message: t("resourcesMCPServerDocumentObjectRequired") };
  }
  const payload = mcpServerPayloadFromDocument(parsed);
  if (!payload) {
    return { kind: "structure", message: t("resourcesMCPServerDocumentInvalidShape") };
  }
  return { kind: "valid", payload };
}

function JSONConfigEditor({
  hideLabel = false,
  invalid = false,
  label,
  minRows = 12,
  onChange,
  value,
}: {
  hideLabel?: boolean;
  invalid?: boolean;
  label: string;
  minRows?: number;
  onChange: (value: string) => void;
  value: string;
}) {
  const editorId = useId();
  const editorParentRef = useRef<HTMLDivElement | null>(null);
  const editorViewRef = useRef<EditorView | null>(null);
  const initialValueRef = useRef(value);
  const onChangeRef = useRef(onChange);
  const minHeight = `${Math.max(minRows, 6) * 19.2 + 28}px`;

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useEffect(() => {
    if (!editorParentRef.current) {
      return;
    }
    const view = new EditorView({
      state: EditorState.create({
        doc: initialValueRef.current,
        extensions: [
          ...jsonEditorExtensions,
          EditorView.contentAttributes.of({
            "aria-label": label,
            id: editorId,
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              onChangeRef.current(update.state.doc.toString());
            }
          }),
        ],
      }),
      parent: editorParentRef.current,
    });
    editorViewRef.current = view;
    return () => {
      editorViewRef.current = null;
      view.destroy();
    };
  }, [editorId, label]);

  useEffect(() => {
    const view = editorViewRef.current;
    if (!view) {
      return;
    }
    const current = view.state.doc.toString();
    if (current === value) {
      return;
    }
    view.dispatch({
      changes: {
        from: 0,
        insert: value,
        to: current.length,
      },
    });
  }, [value]);

  return (
    <div
      className={`hub-json-editor${invalid ? " is-invalid" : ""}`}
      style={{ "--hub-json-editor-min-height": minHeight } as CSSProperties}
    >
      <label className={`hub-json-editor-label${hideLabel ? " sr-only" : ""}`} htmlFor={editorId}>
        {label}
      </label>
      <div className="hub-json-editor-shell" ref={editorParentRef} aria-invalid={invalid || undefined} />
    </div>
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
    mcpServers = [],
    selectedTemplate,
    selectedTemplateId,
    selectedSkill,
    selectedSkillPath,
    selectedMCPServer,
    selectedResourceType = "template",
    loaded,
    error,
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
    mcpStateError = "",
    mcpStateLoading = false,
    mcpMutationBusy = false,
    mcpMutationError = "",
    mcpCreateDialogOpen = false,
    onSelectWorkspaceFile,
    onToggleWorkspaceDir,
    workspaceEntries = EMPTY_WORKSPACE_ENTRIES,
    workspaceTreeLoading = false,
    loadingWorkspaceDirs,
    onSelectSkillFile,
    onDeleteSkill,
    onCreateMCP,
    onDeleteMCP,
    onDeleteTemplate,
    onMCPCreateDialogOpenChange,
    onUpdateMCP,
    deleteBusy = false,
    skillDeleteBusy = false,
  } = hub?.detailPaneProps ?? EMPTY_HUB_DETAIL_PROPS;
  const canDeleteTemplate = isDeletableHubTemplate(selectedTemplate);
  const canDeleteSkill = Boolean(selectedSkill && !isReadonlySkill(selectedSkill));
  const skillEntries = skillTree?.entries ?? EMPTY_WORKSPACE_ENTRIES;
  const activeResourceType = useMemo(() => {
    if (selectedResourceType === "mcp") {
      return "mcp";
    }
    if (selectedResourceType === "skill") {
      return "skill";
    }
    if (templates.length) {
      return "template";
    }
    if (mcpServers.length) {
      return "mcp";
    }
    if (skills.length) {
      return "skill";
    }
    return "template";
  }, [mcpServers.length, selectedResourceType, skills.length, templates.length]);
  const [isInspectorScrolling, setIsInspectorScrolling] = useState(false);
  const [deleteSkillDialogOpen, setDeleteSkillDialogOpen] = useState(false);
  const [mcpDeleteDialogOpen, setMCPDeleteDialogOpen] = useState(false);
  const [mcpDraftDocument, setMCPDraftDocument] = useState(DEFAULT_MCP_SERVER_DOCUMENT);
  const [mcpDetailDocument, setMCPDetailDocument] = useState("");
  const [mcpDetailError, setMCPDetailError] = useState("");
  const [mcpFormError, setMCPFormError] = useState("");
  const inspectorScrollTimerRef = useRef<number | null>(null);
  useEffect(() => {
    if (mcpCreateDialogOpen) {
      setMCPDraftDocument(DEFAULT_MCP_SERVER_DOCUMENT);
      setMCPFormError("");
    }
  }, [mcpCreateDialogOpen]);
  useEffect(() => {
    if (!selectedMCPServer) {
      setMCPDetailDocument("");
      setMCPDetailError("");
      return;
    }
    setMCPDetailDocument(formatMCPServerDocument(selectedMCPServer.name, selectedMCPServer.config));
    setMCPDetailError("");
  }, [selectedMCPServer]);
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

  async function handleSaveMCP() {
    const result = parseMCPServerDocument(mcpDraftDocument, t);
    if (result.kind !== "valid") {
      setMCPFormError(result.kind === "structure" ? result.message : "");
      return;
    }
    const saved = await onCreateMCP?.(result.payload);
    if (saved) {
      closeMCPFormDialog();
    }
  }

  async function handleSaveMCPDetail() {
    if (!selectedMCPServer) {
      return;
    }
    const result = parseMCPServerDocument(mcpDetailDocument, t);
    if (result.kind !== "valid") {
      setMCPDetailError(result.kind === "structure" ? result.message : "");
      return;
    }
    const saved = await onUpdateMCP?.(selectedMCPServer.name, result.payload);
    if (saved) {
      setMCPDetailError("");
    }
  }

  async function handleDeleteMCPConfirm() {
    const deleted = await onDeleteMCP?.(selectedMCPServer);
    if (deleted) {
      setMCPDeleteDialogOpen(false);
    }
  }

  function handleMCPDraftDocumentChange(value: string) {
    setMCPDraftDocument(value);
    const result = parseMCPServerDocument(value, t);
    setMCPFormError(result.kind === "structure" ? result.message : "");
  }

  function handleMCPDetailDocumentChange(value: string) {
    setMCPDetailDocument(value);
    const result = parseMCPServerDocument(value, t);
    setMCPDetailError(result.kind === "structure" ? result.message : "");
  }

  function closeMCPFormDialog() {
    setMCPFormError("");
    onMCPCreateDialogOpenChange?.(false);
  }

  return (
    <section className="entity-pane hub-detail-pane">
      {error ? <div className="form-error">{error}</div> : null}
      {!loaded && !error ? (
        <div className="workspace-empty">{t("resourcesLoading")}</div>
      ) : templates.length === 0 && skills.length === 0 && mcpServers.length === 0 ? (
        <div className="empty-state shell-empty-state hub-empty-state">
          <span className="rich-empty-mark" aria-hidden="true">
            *
          </span>
          <strong>{t("resourcesEmpty")}</strong>
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
                    <div className="hub-inspector-copy">
                      <div className="hub-inspector-title-row">
                        <span className="hub-inspector-title-icon" aria-hidden="true">
                          <ModelsIcon />
                        </span>
                        <h2>{selectedTemplate.name || selectedTemplate.id}</h2>
                        <div className="hub-inspector-badge-row">
                          <span className="mini-badge template-runtime-badge">
                            {selectedTemplate.runtime_kind || selectedTemplate.workspace?.kind
                              ? formatRuntimeKindLabel(
                                  selectedTemplate.runtime_kind || selectedTemplate.workspace?.kind,
                                  t,
                                )
                              : "-"}
                          </span>
                          <span className="mini-badge template-source-badge">
                            <span className="template-source-badge-dot" aria-hidden="true"></span>
                            {localizeTemplateSourceTag(selectedTemplate.source?.name, locale)}
                          </span>
                        </div>
                      </div>
                      <p>{selectedTemplate.description || selectedTemplate.id}</p>
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
                        {t("resourcesDeleteTemplate")}
                      </Button>
                    ) : null}
                  </div>
                </div>
              </div>

              <div className="hub-inspector-grid">
                <div className="hub-inspector-field">
                  <span>{t("resourcesSourceLabel")}</span>
                  <strong>{localizeTemplateSourceTag(selectedTemplate.source?.name, locale)}</strong>
                </div>
                <div className="hub-inspector-field">
                  <span>{t("resourcesRuntimeLabel")}</span>
                  <strong>
                    {selectedTemplate.runtime_kind ? formatRuntimeKindLabel(selectedTemplate.runtime_kind, t) : "-"}
                  </strong>
                </div>
                <div className="hub-inspector-field">
                  <span>{t("resourcesImageLabel")}</span>
                  <strong className="hub-field-value-multiline">{selectedTemplate.image || "-"}</strong>
                </div>
                <div className="hub-inspector-field">
                  <span>{t("resourcesUpdatedAtLabel")}</span>
                  <strong>{formatHubDateTime(selectedTemplate.updated_at, locale)}</strong>
                </div>
              </div>

              <div className="hub-workspace-block">
                <span className="hub-section-label">{t("resourcesWorkspaceTemplateLabel")}</span>
                <div className="hub-workspace-panels">
                  <WorkspaceFileTree
                    key={selectedTemplateId}
                    className="hub-workspace-tree"
                    entries={workspaceEntries}
                    loading={workspaceTreeLoading}
                    loadingText={t("resourcesWorkspaceLoading")}
                    emptyText={t("resourcesWorkspacePreviewHint")}
                    selectedPath={selectedWorkspacePath}
                    loadingPaths={loadingWorkspaceDirs}
                    onSelectFile={onSelectWorkspaceFile}
                    onToggleDir={onToggleWorkspaceDir}
                  />
                  <WorkspaceFilePreview
                    className="hub-workspace-preview"
                    file={workspaceFile}
                    loading={workspaceFileLoading}
                    error={workspaceFileError}
                    loadingText={t("resourcesWorkspaceFileLoading")}
                    emptyTitle={t("resourcesWorkspacePreviewTitle")}
                    emptyHint={t("resourcesWorkspacePreviewHint")}
                    emptyIcon={<HubPreviewEmptyIcon />}
                    binaryText={t("resourcesWorkspaceBinary")}
                    emptyFileText={t("resourcesWorkspaceEmptyFile")}
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
                    <div className="hub-inspector-copy">
                      <div className="hub-inspector-title-row">
                        <span className="hub-inspector-title-icon" aria-hidden="true">
                          <FileCode2 size={18} strokeWidth={2} />
                        </span>
                        <h2>{selectedSkill.name}</h2>
                      </div>
                      <p>{selectedSkill.description || selectedSkill.name}</p>
                    </div>
                  </div>
                  {canDeleteSkill ? (
                    <div className="hub-template-actions">
                      <Button
                        className="hub-skill-delete-button"
                        variant="outlineDanger"
                        size="md"
                        disabled={skillDeleteBusy}
                        onClick={() => setDeleteSkillDialogOpen(true)}
                      >
                        {t("resourcesDeleteSkill")}
                      </Button>
                    </div>
                  ) : null}
                </div>
              </div>

              <div className="hub-workspace-block">
                <div className="hub-workspace-panels">
                  <WorkspaceFileTree
                    className="hub-workspace-tree"
                    entries={skillEntries}
                    loading={skillTreeLoading}
                    loadingText={t("resourcesSkillFilesLoading")}
                    emptyText={skillTreeError || t("resourcesSkillFilesEmpty")}
                    selectedPath={selectedSkillPath}
                    onSelectFile={onSelectSkillFile}
                  />
                  <WorkspaceFilePreview
                    className="hub-workspace-preview"
                    file={skillFile}
                    loading={skillFileLoading}
                    error={skillFileError}
                    loadingText={t("resourcesWorkspaceFileLoading")}
                    emptyTitle={t("resourcesSkillPreviewTitle")}
                    emptyHint={t("resourcesSkillPreviewHint")}
                    emptyIcon={<HubPreviewEmptyIcon />}
                    binaryText={t("resourcesWorkspaceBinary")}
                    emptyFileText={t("resourcesWorkspaceEmptyFile")}
                    previewText={t("workspacePreviewPreviewTab")}
                    codeText={t("workspacePreviewCodeTab")}
                    viewToggleLabel={t("workspacePreviewViewMode")}
                    closeText={t("close")}
                    truncatedText={t("workspacePreviewTruncated")}
                  />
                </div>
              </div>
            </>
          ) : activeResourceType === "mcp" && selectedMCPServer ? (
            <>
              <div className="hub-inspector-hero">
                <div className="hub-inspector-hero-row">
                  <div className="hub-inspector-brand">
                    <div className="hub-inspector-copy">
                      <div className="hub-inspector-title-row">
                        <span className="hub-inspector-title-icon" aria-hidden="true">
                          <Server size={18} strokeWidth={2} />
                        </span>
                        <h2>{selectedMCPServer.name}</h2>
                      </div>
                      <p>
                        {selectedMCPServer.description ||
                          mcpServerDescription(selectedMCPServer.config) ||
                          selectedMCPServer.name}
                      </p>
                    </div>
                  </div>
                  <div className="hub-template-actions">
                    <Button variant="primary" size="md" loading={mcpMutationBusy} onClick={handleSaveMCPDetail}>
                      {mcpMutationBusy ? t("resourcesMCPSaving") : t("resourcesMCPSave")}
                    </Button>
                    <Button
                      variant="outlineDanger"
                      size="md"
                      disabled={mcpMutationBusy}
                      onClick={() => setMCPDeleteDialogOpen(true)}
                    >
                      <Trash2 size={16} strokeWidth={2} />
                      <span>{t("resourcesMCPDelete")}</span>
                    </Button>
                  </div>
                </div>
              </div>

              {mcpStateError || mcpMutationError ? (
                <div className="form-error">{mcpStateError || mcpMutationError}</div>
              ) : null}
              {mcpStateLoading ? <div className="workspace-empty">{t("resourcesMCPLoading")}</div> : null}

              <div className="hub-workspace-block mcp-server-document-block">
                <JSONConfigEditor
                  label={t("resourcesMCPServerDocumentLabel")}
                  value={mcpDetailDocument}
                  onChange={handleMCPDetailDocumentChange}
                  invalid={Boolean(mcpDetailError)}
                  minRows={12}
                />
                {mcpDetailError ? <div className="form-error hub-json-editor-error">{mcpDetailError}</div> : null}
              </div>
            </>
          ) : (
            <div className="empty-state shell-empty-state hub-empty-state">
              <span className="rich-empty-mark" aria-hidden="true">
                *
              </span>
              <strong>
                {activeResourceType === "mcp"
                  ? t("resourcesMCPEmpty")
                  : activeResourceType === "skill"
                    ? t("resourcesSkillsEmpty")
                    : templates.length || skills.length || mcpServers.length
                      ? t("resourcesLoading")
                      : t("resourcesEmpty")}
              </strong>
            </div>
          )}
        </div>
      )}
      <DialogRoot open={deleteSkillDialogOpen} onOpenChange={setDeleteSkillDialogOpen}>
        <DialogContent className="hub-skill-delete-dialog">
          <DialogHeader className="hub-skill-delete-dialog-header">
            <div className="hub-skill-delete-dialog-copy">
              <DialogTitle>{t("resourcesDeleteSkill")}</DialogTitle>
              <DialogDescription>
                {t("resourcesDeleteSkillConfirmMessage", { name: selectedSkill?.name || "" })}
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
              {t("resourcesDeleteSkillConfirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot
        open={mcpCreateDialogOpen}
        onOpenChange={(open) => {
          if (open) {
            setMCPDraftDocument(DEFAULT_MCP_SERVER_DOCUMENT);
            setMCPFormError("");
            onMCPCreateDialogOpenChange?.(true);
          } else {
            closeMCPFormDialog();
          }
        }}
      >
        <DialogContent className="mcp-dialog">
          <DialogHeader className="hub-skill-delete-dialog-header">
            <div className="hub-skill-delete-dialog-copy">
              <DialogTitle>{t("resourcesMCPCreateTitle")}</DialogTitle>
              <DialogDescription>{t("resourcesMCPFormDescription")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <DialogBody className="mcp-form">
            <JSONConfigEditor
              hideLabel
              label={t("resourcesMCPServerDocumentJSONLabel")}
              value={mcpDraftDocument}
              onChange={handleMCPDraftDocumentChange}
              invalid={Boolean(mcpFormError)}
              minRows={12}
            />
            {mcpFormError || mcpMutationError ? (
              <div className="form-error hub-json-editor-error">{mcpFormError || mcpMutationError}</div>
            ) : null}
          </DialogBody>
          <DialogFooter className="hub-skill-delete-dialog-actions">
            <Button variant="secondaryGray" size="sm" disabled={mcpMutationBusy} onClick={closeMCPFormDialog}>
              {t("cancel")}
            </Button>
            <Button variant="primary" size="sm" loading={mcpMutationBusy} onClick={handleSaveMCP}>
              {mcpMutationBusy ? t("resourcesMCPSaving") : t("resourcesMCPSave")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot open={mcpDeleteDialogOpen} onOpenChange={setMCPDeleteDialogOpen}>
        <DialogContent className="hub-skill-delete-dialog">
          <DialogHeader className="hub-skill-delete-dialog-header">
            <div className="hub-skill-delete-dialog-copy">
              <DialogTitle>{t("resourcesMCPDelete")}</DialogTitle>
              <DialogDescription>
                {t("resourcesMCPDeleteConfirmMessage", { name: selectedMCPServer?.name || "" })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {mcpMutationError ? <div className="form-error">{mcpMutationError}</div> : null}
          <DialogFooter className="hub-skill-delete-dialog-actions">
            <Button
              variant="secondaryGray"
              size="sm"
              disabled={mcpMutationBusy}
              onClick={() => setMCPDeleteDialogOpen(false)}
            >
              {t("cancel")}
            </Button>
            <Button variant="danger" size="sm" loading={mcpMutationBusy} onClick={handleDeleteMCPConfirm}>
              {t("resourcesDeleteSkillConfirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </section>
  );
}
