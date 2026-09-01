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
import {
  BookOpen,
  CheckCircle2,
  CloudDownload,
  ExternalLink,
  FileCode2,
  RefreshCw,
  Server,
  Trash2,
} from "lucide-react";
import { formatRuntimeKindLabel } from "@/models/agents";
import {
  canPublishHubTemplateToCommunity,
  formatHubDateTime,
  hubTemplateFullName,
  hubTemplateReviewState,
  isDeletableHubTemplate,
  isHubTemplateMemoryEnabled,
} from "@/models/hubWorkspace";
import {
  formatMCPServerDocument,
  mcpManagedKnowledgeBaseSource,
  mcpServerDescription,
  mcpServerPayloadFromDocument,
  mcpServersFromTemplateDocument,
  mcpToolParameters,
} from "@/models/mcp";
import type { MCPProbeResult, MCPServerPayload, MCPServerSourceStatus, RemoteMCPServer } from "@/models/mcp";
import { WorkspaceFilePreview, WorkspaceFileTree } from "@/components/business/WorkspaceFileTree";
import { localizeTemplateSourceTag } from "@/shared/i18n";
import { ModelsIcon } from "@/components/ui/Icons";
import {
  Button,
  Checkbox,
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
import type { RemoteKnowledgeBase } from "@/models/knowledgeBases";
import { isReadonlySkill } from "@/models/skillhub";
import type { SkillFile, SkillSummary, SkillTree } from "@/models/skillhub";
import type { WorkspaceEntry, WorkspaceFile } from "@/models/workspace";
import { RemoteMCPList } from "./RemoteMCPList";
import styles from "./HubDetailPane.module.css";

type ModuleClassValue = string | false | null | undefined;

function moduleClassNames(...values: ModuleClassValue[]): string {
  return values
    .filter((value): value is string => Boolean(value))
    .flatMap((value) => value.split(/\s+/))
    .filter(Boolean)
    .map((value) => styles[value] || value)
    .join(" ");
}

const EMPTY_WORKSPACE_ENTRIES: readonly WorkspaceEntry[] = [];
type TemplateDetailTabID = "profile" | "instructions" | "memory" | "skills" | "mcp";
type MCPCreateMode = "manual" | "remote";

type TemplateSkillSummary = {
  description: string;
  name: string;
};

function knowledgeBaseUnavailableText(reason: string, t: TranslateFn): string {
  switch (String(reason || "").trim()) {
    case "missing_content":
    case "content_not_ready":
      return t("resourcesKnowledgeBaseNoActiveContent");
    case "mcp_not_ready":
      return t("resourcesKnowledgeBaseMCPNotReady");
    case "remote_state_unavailable":
      return t("resourcesKnowledgeBaseRemoteUnavailable");
    default:
      return reason;
  }
}

function formatKnowledgeBaseMCPPreview(server: MCPServer | null): string {
  if (!server) {
    return "";
  }
  const config = JSON.parse(
    JSON.stringify(server.config, (key, value) =>
      key.toLowerCase() === "authorization" ? "Bearer ${OPENCSG_TOKEN}" : value,
    ),
  ) as MCPServer["config"];
  return formatMCPServerDocument(server.name, config);
}

function templateSectionEntries(
  entries: readonly WorkspaceEntry[],
  section: "instructions" | "memories" | "skills" | "mcps",
): WorkspaceEntry[] {
  const prefix = `${section}/`;
  return entries
    .filter((entry) => entry.path.startsWith(prefix))
    .map((entry) => ({
      ...entry,
      path: entry.path.slice(prefix.length),
      depth: Math.max(0, (entry.depth ?? 1) - 1),
    }));
}

function templateSectionPath(path: string, section: "instructions" | "memories" | "skills" | "mcps"): string {
  const prefix = `${section}/`;
  return path.startsWith(prefix) ? path.slice(prefix.length) : "";
}

function firstTemplateSectionFile(
  entries: readonly WorkspaceEntry[],
  section: "instructions" | "memories" | "skills" | "mcps",
  preferredName?: string,
): string {
  const prefix = `${section}/`;
  const files = entries
    .filter((entry) => entry.type === "file" && entry.path.startsWith(prefix))
    .map((entry) => entry.path);
  if (!files.length) {
    return "";
  }
  if (preferredName) {
    const preferred = files.find(
      (path) => path.endsWith(`/${preferredName}`) || path === `${section}/${preferredName}`,
    );
    if (preferred) {
      return preferred;
    }
  }
  return files[0] || "";
}

function parseSkillMarkdownSummary(content: string): Pick<TemplateSkillSummary, "description"> {
  const frontmatter = /^---\s*\n([\s\S]*?)\n---/.exec(content);
  if (frontmatter) {
    const description = parseYamlScalar(frontmatter[1] || "", "description");
    if (description) {
      return { description };
    }
  }
  const firstParagraph =
    content
      .replace(/^---\s*\n[\s\S]*?\n---/, "")
      .split(/\n{2,}/)
      .map((block) =>
        block
          .split("\n")
          .map((line) => line.replace(/^#+\s*/, "").trim())
          .filter(Boolean)
          .join(" "),
      )
      .find(Boolean) || "";
  return { description: firstParagraph };
}

function parseYamlScalar(source: string, key: string): string {
  const pattern = new RegExp(`^${key}:\\s*(.*)$`, "im");
  const match = pattern.exec(source);
  if (!match) {
    return "";
  }
  return String(match[1] || "")
    .trim()
    .replace(/^["']|["']$/g, "");
}

function templateSkillSummaries(
  entries: readonly WorkspaceEntry[],
  workspaceFile: WorkspaceFile | null,
  workspaceFiles: Readonly<Record<string, WorkspaceFile>>,
): TemplateSkillSummary[] {
  const skillNames = Array.from(
    new Set(
      entries
        .filter((entry) => entry.path.startsWith("skills/"))
        .map((entry) => entry.path.split("/")[1] || "")
        .filter(Boolean),
    ),
  ).sort((left, right) => left.localeCompare(right));

  return skillNames.map((name) => {
    const skillPath = `skills/${name}/SKILL.md`;
    const summaryFile = workspaceFiles[skillPath] || (workspaceFile?.path === skillPath ? workspaceFile : null);
    const loadedDescription =
      summaryFile && !summaryFile.binary ? parseSkillMarkdownSummary(summaryFile.content || "").description : "";
    return {
      name,
      description: loadedDescription,
    };
  });
}

function templateMCPServerSummaries(
  workspaceFile: WorkspaceFile | null,
  workspaceFiles: Readonly<Record<string, WorkspaceFile>>,
) {
  const mcpFile = workspaceFiles["mcps/mcp.json"] || (workspaceFile?.path === "mcps/mcp.json" ? workspaceFile : null);
  if (!mcpFile || mcpFile.binary) {
    return [];
  }
  try {
    return mcpServersFromTemplateDocument(JSON.parse(mcpFile.content || ""));
  } catch {
    return [];
  }
}

function MCPProbePanel({ result, t }: { result: MCPProbeResult; t: TranslateFn }) {
  const serverLabel = result.serverInfo?.title || result.serverInfo?.name || "";
  return (
    <section className={moduleClassNames("mcp-probe-panel")} aria-live="polite">
      <div className={moduleClassNames("mcp-probe-header")}>
        <span className={moduleClassNames("mcp-probe-success-icon")} aria-hidden="true">
          <CheckCircle2 size={18} strokeWidth={2.2} />
        </span>
        <div className={moduleClassNames("mcp-probe-heading-copy")}>
          <strong>{t("resourcesMCPTestSuccess")}</strong>
          <span>
            {result.toolsSupported
              ? t("resourcesMCPTestSuccessSummary", { count: result.tools.length })
              : t("resourcesMCPToolsUnsupported")}
          </span>
        </div>
        <div className={moduleClassNames("mcp-probe-meta")}>
          {serverLabel ? (
            <span>
              {serverLabel}
              {result.serverInfo?.version ? ` ${result.serverInfo.version}` : ""}
            </span>
          ) : null}
          {result.protocolVersion ? (
            <span>{t("resourcesMCPProtocolVersion", { version: result.protocolVersion })}</span>
          ) : null}
          <span>{t("resourcesMCPTestDuration", { duration: result.durationMs })}</span>
        </div>
      </div>

      {result.toolsSupported ? (
        <div className={moduleClassNames("mcp-tools-section")}>
          <div className={moduleClassNames("mcp-tools-heading")}>
            <strong>{t("resourcesMCPToolsTitle")}</strong>
            <span>{result.tools.length}</span>
          </div>
          {result.tools.length ? (
            <div className={moduleClassNames("mcp-tools-list")}>
              {result.tools.map((tool) => {
                const parameters = mcpToolParameters(tool);
                const displayName = tool.title || tool.name;
                return (
                  <article key={tool.name} className={moduleClassNames("mcp-tool-card")}>
                    <div className={moduleClassNames("mcp-tool-title-row")}>
                      <strong>{displayName}</strong>
                      {displayName !== tool.name ? <code>{tool.name}</code> : null}
                    </div>
                    {tool.description ? <p>{tool.description}</p> : null}
                    {parameters.length ? (
                      <div className={moduleClassNames("mcp-tool-parameters")}>
                        {parameters.map((parameter) => (
                          <span
                            key={parameter.name}
                            className={moduleClassNames(
                              "mcp-tool-parameter",
                              parameter.required && "mcp-tool-parameter-required",
                            )}
                            title={parameter.description}
                          >
                            <code>{parameter.name}</code>
                            <span>{parameter.type}</span>
                            <em>
                              {parameter.required
                                ? t("resourcesMCPToolParameterRequired")
                                : t("resourcesMCPToolParameterOptional")}
                            </em>
                          </span>
                        ))}
                      </div>
                    ) : null}
                  </article>
                );
              })}
            </div>
          ) : (
            <div className={moduleClassNames("mcp-tools-empty")}>{t("resourcesMCPToolsEmpty")}</div>
          )}
          {result.truncated ? (
            <div className={moduleClassNames("mcp-tools-truncated")}>
              {t("resourcesMCPToolsTruncated", { count: result.tools.length })}
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

const managedInstructionsStart = "<!-- BEGIN CSGCLAW-INSTRUCTIONS (auto-generated; do not edit) -->";
const managedInstructionsEnd = "<!-- END CSGCLAW-INSTRUCTIONS -->";

function extractManagedAgentInstructions(document: string): string {
  const blockStart = document.indexOf(managedInstructionsStart);
  const blockEnd = blockStart < 0 ? -1 : document.indexOf(managedInstructionsEnd, blockStart);
  if (blockStart < 0 || blockEnd < 0) return "";
  const block = document.slice(blockStart, blockEnd);
  const heading = "# Agent Instructions\n\n";
  const bodyStart = block.indexOf(heading);
  if (bodyStart < 0) return "";
  const body = block.slice(bodyStart + heading.length);
  const nextHeading = body.search(/\n# (?:Managed Runtime Instructions|CSGClaw Rules)/);
  return (nextHeading >= 0 ? body.slice(0, nextHeading) : body).trim();
}

function replaceManagedAgentInstructions(document: string, instructions: string): string {
  const block = `${managedInstructionsStart}\n\n${instructions.trim() ? `# Agent Instructions\n\n${instructions.trim()}\n\n` : ""}${managedInstructionsEnd}`;
  const blockStart = document.indexOf(managedInstructionsStart);
  const blockEnd = blockStart < 0 ? -1 : document.indexOf(managedInstructionsEnd, blockStart);
  if (blockStart >= 0 && blockEnd >= 0) {
    return `${document.slice(0, blockStart)}${block}${document.slice(blockEnd + managedInstructionsEnd.length)}`;
  }
  return `${document.trimEnd()}${document.trim() ? "\n\n" : ""}${block}\n`;
}

type HubDetailPaneHub = {
  detailPaneProps: {
    deleteBusy?: boolean;
    detailLoading?: boolean;
    error: string;
    loaded: boolean;
    onDeleteSkill?: (item: SkillSummary | null | undefined) => Promise<boolean> | boolean;
    onCreateMCP?: (payload: MCPServerPayload) => Promise<boolean> | boolean;
    onCheckMCPSource?: () => Promise<MCPServerSourceStatus | null> | MCPServerSourceStatus | null;
    onClearMCPProbe?: () => void;
    onDeleteMCP?: (item: MCPServer | null | undefined) => Promise<boolean> | boolean;
    onDeleteTemplate?: (item: HubTemplate | null | undefined) => unknown;
    onPublishTemplate?: (
      item: HubTemplate | null | undefined,
      deploy?: boolean,
      includeMemory?: boolean,
    ) =>
      | Promise<{ status: "success" } | { status: "partial"; message: string } | null>
      | { status: "success" }
      | { status: "partial"; message: string }
      | null;
    onSelectMCP?: (name: string | null | undefined) => void;
    onProbeMCP?: (payload: MCPServerPayload) => Promise<MCPProbeResult | null> | MCPProbeResult | null;
    onSyncMCPSource?: () => Promise<boolean> | boolean;
    onUpdateMCP?: (currentName: string, payload: MCPServerPayload) => Promise<boolean> | boolean;
    onRetry: () => void | Promise<void>;
    onSelectSkill?: (name: string | null | undefined) => void;
    onSelectSkillFile?: (path: string) => void;
    onSelectTemplate?: (item: HubTemplate | null | undefined) => void;
    onSelectWorkspaceFile: (workspacePath: string) => void;
    onUpdateTemplateInstructions?: (content: string) => boolean | Promise<boolean>;
    onToggleWorkspaceDir?: (workspacePath: string) => void | Promise<void>;
    mcpServers?: readonly MCPServer[];
    mcpStateError?: string;
    mcpStateLoading?: boolean;
    mcpMutationBusy?: boolean;
    mcpMutationError?: string;
    mcpProbeBusy?: boolean;
    mcpProbeError?: string;
    mcpProbeResult?: MCPProbeResult | null;
    mcpSourceBusy?: boolean;
    mcpSourceError?: string;
    mcpSourceStatus?: MCPServerSourceStatus | null;
    mcpSourceSyncBusy?: boolean;
    mcpCreateError?: string;
    mcpCreateDialogOpen?: boolean;
    mcpCreateInitialDocument?: string;
    knowledgeBases?: {
      cancelMCPConfig: () => void;
      confirmMCPConfig: () => Promise<boolean>;
      copyBusyID: string;
      copyError: string;
      items: readonly RemoteKnowledgeBase[];
      loginRequired: boolean;
      loading: boolean;
      loadError: string;
      pendingMCPKnowledgeBase: RemoteKnowledgeBase | null;
      requestMCPConfig: (id: string) => Promise<boolean>;
      search: string;
      selected: RemoteKnowledgeBase | null;
      setSearch: (value: string) => void;
    };
    onMCPCreateDialogOpenChange?: (open: boolean) => void;
    onKnowledgeBaseLogin?: () => void | Promise<void>;
    onInstallRemoteMCP?: (item: RemoteMCPServer) => Promise<boolean> | boolean;
    onLoadMoreRemoteMCPServers?: () => Promise<unknown> | unknown;
    onRefreshRemoteMCPServers?: () => Promise<unknown> | unknown;
    onRemoteMCPServersSearchChange?: (value: string) => void;
    onRemoteMCPVisibleChange?: (visible: boolean) => void;
    remoteMCPInstallBusy?: string;
    remoteMCPServers?: readonly RemoteMCPServer[];
    remoteMCPServersError?: string;
    remoteMCPServersHasMore?: boolean;
    remoteMCPServersLoading?: boolean;
    remoteMCPServersLoadingMore?: boolean;
    remoteMCPServersSearch?: string;
    selectedMCPServer?: MCPServer | null;
    selectedMCPServerName?: string;
    selectedResourceType?: "knowledge" | "mcp" | "skill" | "template";
    selectedSkill: SkillSummary | null;
    selectedSkillPath: string;
    selectedTemplate: HubTemplate | null;
    selectedTemplateId: string;
    selectedWorkspacePath: string;
    skillFile: SkillFile | null;
    skillFileError: string;
    skillFileLoading: boolean;
    skillDeleteBusy?: boolean;
    publishBusy?: boolean;
    publishDisabled?: boolean;
    publishError?: string;
    skills: readonly SkillSummary[];
    skillTree: SkillTree | null;
    skillTreeError: string;
    skillTreeLoading: boolean;
    templates: readonly HubTemplate[];
    workspaceFile: WorkspaceFile | null;
    workspaceFileError: string;
    workspaceFileLoading: boolean;
    workspaceFiles?: Readonly<Record<string, WorkspaceFile>>;
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
  mcpProbeBusy: false,
  mcpProbeError: "",
  mcpProbeResult: null,
  mcpCreateError: "",
  mcpCreateDialogOpen: false,
  mcpCreateInitialDocument: "",
  remoteMCPInstallBusy: "",
  remoteMCPServers: [],
  remoteMCPServersError: "",
  remoteMCPServersHasMore: false,
  remoteMCPServersLoading: false,
  remoteMCPServersLoadingMore: false,
  remoteMCPServersSearch: "",
  selectedMCPServer: null,
  selectedMCPServerName: "",
  onSelectSkillFile: () => {},
  onSelectWorkspaceFile: () => {},
  onToggleWorkspaceDir: () => {},
  onCreateMCP: () => false,
  onCheckMCPSource: () => null,
  onClearMCPProbe: () => {},
  onDeleteMCP: () => false,
  onProbeMCP: () => null,
  onSyncMCPSource: () => false,
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
  workspaceFiles: {},
  workspaceEntries: [],
  workspaceTreeLoading: false,
  loadingWorkspaceDirs: new Set(),
};

const DEFAULT_MCP_SERVER_DOCUMENT =
  '{\n  "mcpServers": {\n    "filesystem": {\n      "command": "npx",\n      "args": ["-y", "@modelcontextprotocol/server-filesystem", "${workspace}"],\n      "startup_timeout_sec": 60\n    }\n  }\n}';

const jsonEditorTheme = EditorView.theme({
  "&": {
    backgroundColor: "transparent",
    color: "var(--hub-json-editor-text, var(--text))",
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
    caretColor: "var(--hub-json-editor-caret)",
    padding: "14px 0",
  },
  ".cm-line": {
    padding: "0 14px",
  },
  ".cm-gutters": {
    backgroundColor: "var(--hub-json-editor-gutter-bg)",
    borderRight: "1px solid var(--hub-json-editor-gutter-border)",
    color: "var(--hub-json-editor-gutter-text)",
    paddingLeft: "4px",
  },
  ".cm-activeLine": {
    backgroundColor: "var(--hub-json-editor-active-line)",
  },
  ".cm-activeLineGutter": {
    backgroundColor: "var(--hub-json-editor-gutter-active-bg)",
    color: "var(--hub-json-editor-gutter-active-text)",
  },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
    backgroundColor: "var(--hub-json-editor-selection)",
  },
  ".cm-lintRange-error": {
    textDecoration: "underline wavy var(--error-600)",
    textDecorationSkipInk: "none",
  },
  ".cm-tooltip": {
    border: "1px solid var(--hub-json-editor-tooltip-border)",
    borderRadius: "var(--radius-md)",
    backgroundColor: "var(--hub-json-editor-tooltip-bg)",
    color: "var(--text)",
    boxShadow: "var(--shadow-lg)",
    fontFamily: "var(--font-sans)",
    fontSize: "12px",
  },
});

const jsonHighlightStyle = HighlightStyle.define([
  { tag: tags.propertyName, color: "var(--hub-json-editor-property)" },
  { tag: tags.string, color: "var(--hub-json-editor-string)" },
  { tag: tags.number, color: "var(--hub-json-editor-number)" },
  { tag: tags.bool, color: "var(--hub-json-editor-literal)" },
  { tag: tags.null, color: "var(--hub-json-editor-literal)" },
  { tag: tags.punctuation, color: "var(--hub-json-editor-punctuation)" },
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
      className={moduleClassNames(`hub-json-editor${invalid ? " is-invalid" : ""}`)}
      style={{ "--hub-json-editor-min-height": minHeight } as CSSProperties}
    >
      <label className={moduleClassNames(`hub-json-editor-label${hideLabel ? " sr-only" : ""}`)} htmlFor={editorId}>
        {label}
      </label>
      <div
        className={moduleClassNames("hub-json-editor-shell")}
        ref={editorParentRef}
        aria-invalid={invalid || undefined}
      />
    </div>
  );
}

function HubPreviewEmptyIcon() {
  return (
    <svg
      className={moduleClassNames("hub-preview-empty-icon")}
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
    workspaceFiles = {},
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
    mcpProbeBusy = false,
    mcpProbeError = "",
    mcpProbeResult = null,
    mcpSourceBusy = false,
    mcpSourceError = "",
    mcpSourceStatus = null,
    mcpSourceSyncBusy = false,
    mcpCreateError = "",
    mcpCreateDialogOpen = false,
    mcpCreateInitialDocument = "",
    knowledgeBases,
    remoteMCPInstallBusy = "",
    remoteMCPServers = [],
    remoteMCPServersError = "",
    remoteMCPServersHasMore = false,
    remoteMCPServersLoading = false,
    remoteMCPServersLoadingMore = false,
    remoteMCPServersSearch = "",
    onSelectWorkspaceFile,
    onToggleWorkspaceDir,
    workspaceEntries = EMPTY_WORKSPACE_ENTRIES,
    workspaceTreeLoading = false,
    onSelectSkillFile,
    onClearMCPProbe,
    onCheckMCPSource,
    onDeleteSkill,
    onCreateMCP,
    onDeleteMCP,
    onDeleteTemplate,
    onPublishTemplate,
    onSelectMCP,
    onMCPCreateDialogOpenChange,
    onKnowledgeBaseLogin,
    onInstallRemoteMCP,
    onLoadMoreRemoteMCPServers,
    onRefreshRemoteMCPServers,
    onRemoteMCPServersSearchChange,
    onRemoteMCPVisibleChange,
    onProbeMCP,
    onSyncMCPSource,
    onUpdateMCP,
    onUpdateTemplateInstructions,
    deleteBusy = false,
    publishBusy = false,
    publishDisabled = false,
    publishError = "",
    skillDeleteBusy = false,
  } = hub?.detailPaneProps ?? EMPTY_HUB_DETAIL_PROPS;
  const canDeleteTemplate = isDeletableHubTemplate(selectedTemplate);
  const canPublishTemplate = canPublishHubTemplateToCommunity(selectedTemplate);
  const templateReview = hubTemplateReviewState(selectedTemplate);
  const canDeleteSkill = Boolean(selectedSkill && !isReadonlySkill(selectedSkill));
  const skillEntries = skillTree?.entries ?? EMPTY_WORKSPACE_ENTRIES;
  const activeResourceType = useMemo(() => {
    if (selectedResourceType === "knowledge") {
      return "knowledge";
    }
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
  const [deleteSkillDialogOpen, setDeleteSkillDialogOpen] = useState(false);
  const [publishSuccessDialogOpen, setPublishSuccessDialogOpen] = useState(false);
  const [publishPartialMessage, setPublishPartialMessage] = useState("");
  const [publishChoiceDialogOpen, setPublishChoiceDialogOpen] = useState(false);
  const [publishTemplateIncludeMemory, setPublishTemplateIncludeMemory] = useState(false);
  const [mcpDeleteDialogOpen, setMCPDeleteDialogOpen] = useState(false);
  const [knowledgeBaseDeleteDialogOpen, setKnowledgeBaseDeleteDialogOpen] = useState(false);
  const [mcpDraftDocument, setMCPDraftDocument] = useState(DEFAULT_MCP_SERVER_DOCUMENT);
  const [mcpDetailDocument, setMCPDetailDocument] = useState("");
  const [mcpDetailError, setMCPDetailError] = useState("");
  const [mcpFormError, setMCPFormError] = useState("");
  const [mcpCreateMode, setMCPCreateMode] = useState<MCPCreateMode>("manual");
  const configuredKnowledgeBaseMCP = useMemo(() => {
    const name = knowledgeBases?.selected?.configuredMCPName;
    return name ? mcpServers.find((server) => server.name === name) || null : null;
  }, [knowledgeBases?.selected?.configuredMCPName, mcpServers]);
  const knowledgeBaseMCPPreview = useMemo(
    () => formatKnowledgeBaseMCPPreview(configuredKnowledgeBaseMCP),
    [configuredKnowledgeBaseMCP],
  );
  const selectedManagedMCPSource = useMemo(
    () => mcpManagedKnowledgeBaseSource(selectedMCPServer?.config),
    [selectedMCPServer?.config],
  );
  const [activeTemplateTab, setActiveTemplateTab] = useState<TemplateDetailTabID>("profile");
  const [templateInstructionsMode, setTemplateInstructionsMode] = useState<"default" | "advanced">("default");
  const [templateInstructionsDraft, setTemplateInstructionsDraft] = useState("");
  const [templateInstructionsSaving, setTemplateInstructionsSaving] = useState(false);
  const templateSection =
    activeTemplateTab === "mcp" ? "mcps" : activeTemplateTab === "memory" ? "memories" : activeTemplateTab;
  const templateImageEnv = selectedTemplate?.image_env || [];
  const templateSectionWorkspaceEntries = useMemo(
    () =>
      templateSection === "instructions" ||
      templateSection === "memories" ||
      templateSection === "skills" ||
      templateSection === "mcps"
        ? templateSectionEntries(workspaceEntries, templateSection)
        : [],
    [templateSection, workspaceEntries],
  );
  const templateSkillCount = useMemo(
    () =>
      new Set(
        templateSectionEntries(workspaceEntries, "skills")
          .map((entry) => entry.path.split("/")[0])
          .filter(Boolean),
      ).size,
    [workspaceEntries],
  );
  const templateSkills = useMemo(
    () => templateSkillSummaries(workspaceEntries, workspaceFile, workspaceFiles),
    [workspaceEntries, workspaceFile, workspaceFiles],
  );
  const templateMCPServers = useMemo(
    () => templateMCPServerSummaries(workspaceFile, workspaceFiles),
    [workspaceFile, workspaceFiles],
  );
  const templateInstructionsFile =
    workspaceFiles["instructions/AGENTS.md"] ||
    (workspaceFile?.path === "instructions/AGENTS.md" ? workspaceFile : null);
  const templateMemoryFile =
    workspaceFiles["memories/memory_summary.md"] ||
    (workspaceFile?.path === "memories/memory_summary.md" ? workspaceFile : null);
  const templateInstructions = templateInstructionsFile?.content || "";
  useEffect(() => {
    setTemplateInstructionsDraft(templateInstructions);
  }, [selectedTemplateId, templateInstructions]);
  const templateCustomInstructions = extractManagedAgentInstructions(templateInstructionsDraft);
  const templateInstructionsValue =
    templateInstructionsMode === "advanced" ? templateInstructionsDraft : templateCustomInstructions;
  const templateInstructionsReadonly = selectedTemplate?.source?.kind !== "local";
  const templateMemoryEnabled = isHubTemplateMemoryEnabled(selectedTemplate);
  useEffect(() => {
    setKnowledgeBaseDeleteDialogOpen(false);
  }, [knowledgeBases?.selected?.id]);
  const templateTabs = useMemo(
    () => [
      { id: "profile" as const, label: t("agentProfileTab") },
      { id: "instructions" as const, label: t("agentInstructions") },
      ...(templateMemoryEnabled ? [{ id: "memory" as const, label: t("agentMemoryTab") }] : []),
      {
        id: "skills" as const,
        label: t("agentProfileSkillsTab"),
        count: templateSkillCount > 0 ? templateSkillCount : undefined,
      },
      {
        id: "mcp" as const,
        label: t("agentProfileMCPTab"),
        count: templateMCPServers.length > 0 ? templateMCPServers.length : undefined,
      },
    ],
    [t, templateMCPServers.length, templateMemoryEnabled, templateSkillCount],
  );
  useEffect(() => {
    setActiveTemplateTab("profile");
  }, [selectedTemplateId]);
  useEffect(() => {
    const directories = new Set<string>(["skills"]);
    if (activeTemplateTab !== "profile") {
      directories.add(
        activeTemplateTab === "mcp" ? "mcps" : activeTemplateTab === "memory" ? "memories" : activeTemplateTab,
      );
    }
    if (activeTemplateTab === "skills") {
      templateSkills.forEach((skill) => directories.add(`skills/${skill.name}`));
    }
    for (const directory of directories) {
      if (workspaceEntries.some((entry) => entry.path === directory && entry.type === "dir")) {
        void onToggleWorkspaceDir?.(directory);
      }
    }
  }, [activeTemplateTab, onToggleWorkspaceDir, templateSkills, workspaceEntries]);
  useEffect(() => {
    if (activeTemplateTab !== "memory" || selectedWorkspacePath === "memories/memory_summary.md") {
      return;
    }
    const memoryFile = firstTemplateSectionFile(workspaceEntries, "memories", "memory_summary.md");
    if (memoryFile) {
      onSelectWorkspaceFile(memoryFile);
    }
  }, [activeTemplateTab, onSelectWorkspaceFile, selectedWorkspacePath, workspaceEntries]);
  useEffect(() => {
    if (mcpCreateDialogOpen) {
      setMCPDraftDocument(mcpCreateInitialDocument || DEFAULT_MCP_SERVER_DOCUMENT);
      setMCPFormError("");
      setMCPCreateMode("manual");
    }
  }, [mcpCreateDialogOpen, mcpCreateInitialDocument]);
  useEffect(() => {
    if (!selectedMCPServer) {
      setMCPDetailDocument("");
      setMCPDetailError("");
      return;
    }
    setMCPDetailDocument(formatMCPServerDocument(selectedMCPServer.name, selectedMCPServer.config));
    setMCPDetailError("");
  }, [selectedMCPServer]);
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

  async function handleProbeMCPDetail() {
    const result = parseMCPServerDocument(mcpDetailDocument, t);
    if (result.kind !== "valid") {
      setMCPDetailError(result.kind === "structure" ? result.message : "");
      return;
    }
    setMCPDetailError("");
    await onProbeMCP?.(result.payload);
  }

  async function handleCheckMCPSource() {
    await onCheckMCPSource?.();
  }

  async function handleSyncMCPSource() {
    await onSyncMCPSource?.();
  }

  async function handleDeleteMCPConfirm() {
    const deleted = await onDeleteMCP?.(selectedMCPServer);
    if (deleted) {
      setMCPDeleteDialogOpen(false);
    }
  }

  async function handleDeleteKnowledgeBaseMCPConfirm() {
    const deleted = await onDeleteMCP?.(configuredKnowledgeBaseMCP);
    if (deleted) {
      setKnowledgeBaseDeleteDialogOpen(false);
    }
  }

  async function handlePublishTemplate(deploy = false) {
    const result = await onPublishTemplate?.(selectedTemplate, deploy, publishTemplateIncludeMemory);
    if (result?.status === "success") {
      setPublishPartialMessage("");
      setPublishChoiceDialogOpen(false);
      setPublishSuccessDialogOpen(true);
    } else if (result?.status === "partial") {
      setPublishPartialMessage(result.message);
      setPublishChoiceDialogOpen(false);
      setPublishSuccessDialogOpen(true);
    }
  }

  function handleMCPDraftDocumentChange(value: string) {
    setMCPDraftDocument(value);
    const result = parseMCPServerDocument(value, t);
    setMCPFormError(result.kind === "structure" ? result.message : "");
  }

  function handleMCPDetailDocumentChange(value: string) {
    setMCPDetailDocument(value);
    onClearMCPProbe?.();
    const result = parseMCPServerDocument(value, t);
    setMCPDetailError(result.kind === "structure" ? result.message : "");
  }

  function closeMCPFormDialog() {
    setMCPFormError("");
    setMCPCreateMode("manual");
    onMCPCreateDialogOpenChange?.(false);
  }

  function selectTemplateTab(tab: TemplateDetailTabID) {
    setActiveTemplateTab(tab);
    if (tab !== "profile") {
      const selectedFile =
        tab === "skills"
          ? firstTemplateSectionFile(workspaceEntries, "skills", "SKILL.md")
          : tab === "mcp"
            ? firstTemplateSectionFile(workspaceEntries, "mcps", "mcp.json")
            : tab === "memory"
              ? firstTemplateSectionFile(workspaceEntries, "memories", "memory_summary.md")
              : firstTemplateSectionFile(workspaceEntries, "instructions", "AGENTS.md");
      onSelectWorkspaceFile(selectedFile);
    }
  }

  async function saveTemplateInstructions() {
    const content =
      templateInstructionsMode === "advanced"
        ? templateInstructionsDraft
        : replaceManagedAgentInstructions(templateInstructionsDraft, templateCustomInstructions);
    setTemplateInstructionsSaving(true);
    try {
      await onUpdateTemplateInstructions?.(content);
    } finally {
      setTemplateInstructionsSaving(false);
    }
  }

  return (
    <section className={moduleClassNames("entity-pane hub-detail-pane")}>
      {error ? <div className={moduleClassNames("form-error")}>{error}</div> : null}
      {!loaded && !error ? (
        <div className={moduleClassNames("workspace-empty")}>{t("resourcesLoading")}</div>
      ) : activeResourceType !== "knowledge" &&
        templates.length === 0 &&
        skills.length === 0 &&
        mcpServers.length === 0 ? (
        <div className={moduleClassNames("empty-state shell-empty-state hub-empty-state")}>
          <span className={moduleClassNames("rich-empty-mark")} aria-hidden="true">
            *
          </span>
          <strong>{t("resourcesEmpty")}</strong>
        </div>
      ) : (
        <div className={moduleClassNames("hub-workbench hub-inspector-panel")}>
          {activeResourceType === "knowledge" ? (
            <>
              <div className={moduleClassNames("hub-inspector-hero")}>
                <div className={moduleClassNames("hub-inspector-hero-row")}>
                  <div className={moduleClassNames("hub-inspector-brand")}>
                    <div className={moduleClassNames("hub-inspector-copy")}>
                      <div className={moduleClassNames("hub-inspector-title-row")}>
                        <span className={moduleClassNames("hub-inspector-title-icon")} aria-hidden="true">
                          <BookOpen size={18} strokeWidth={2} />
                        </span>
                        <h2>{knowledgeBases?.selected?.name || t("resourcesKnowledgeBasesLabel")}</h2>
                        {knowledgeBases?.selected ? (
                          <span
                            className={moduleClassNames(
                              `mini-badge knowledge-base-status ${knowledgeBases.selected.availability}`,
                            )}
                          >
                            {knowledgeBases.selected.configuredMCPName
                              ? t("resourcesKnowledgeBaseAdded")
                              : knowledgeBases.selected.availability === "available"
                                ? t("resourcesKnowledgeBaseAvailable")
                                : t("resourcesKnowledgeBaseUnavailable")}
                          </span>
                        ) : null}
                      </div>
                      <p>{knowledgeBases?.selected?.description || t("resourcesKnowledgeBasesDescription")}</p>
                      {knowledgeBases?.selected ? (
                        <div className={moduleClassNames("knowledge-base-badges")}>
                          <span className={moduleClassNames("mini-badge knowledge-base-source-badge")}>
                            {t("resourcesKnowledgeBaseAgenticHub")}
                          </span>
                          {knowledgeBases.selected.configuredMCPName ? (
                            <span className={moduleClassNames("mini-badge knowledge-base-mcp-badge")}>
                              {t("resourcesKnowledgeBaseMCPServerBadge")}
                            </span>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                  </div>
                  {knowledgeBases?.selected ? (
                    <div className={moduleClassNames("hub-template-actions")}>
                      {knowledgeBases.selected.configuredMCPName ? (
                        <>
                          <Button
                            variant="secondaryGray"
                            size="md"
                            onClick={() => onSelectMCP?.(knowledgeBases.selected?.configuredMCPName)}
                          >
                            <ExternalLink size={16} strokeWidth={2} aria-hidden="true" />
                            {t("resourcesKnowledgeBaseViewMCP")}
                          </Button>
                          <Button
                            variant="danger"
                            size="md"
                            disabled={!configuredKnowledgeBaseMCP}
                            onClick={() => setKnowledgeBaseDeleteDialogOpen(true)}
                          >
                            <Trash2 size={16} strokeWidth={2} aria-hidden="true" />
                            {t("resourcesKnowledgeBaseRemoveMCP")}
                          </Button>
                        </>
                      ) : (
                        <Button
                          variant="primary"
                          size="md"
                          disabled={knowledgeBases.selected.availability !== "available"}
                          onClick={() => void knowledgeBases.requestMCPConfig(knowledgeBases.selected?.id || "")}
                        >
                          {t("resourcesKnowledgeBaseAddMCP")}
                        </Button>
                      )}
                    </div>
                  ) : null}
                </div>
                {knowledgeBases?.selected?.unavailableReason ? (
                  <div className={moduleClassNames("form-error")}>
                    {knowledgeBaseUnavailableText(knowledgeBases.selected.unavailableReason, t)}
                  </div>
                ) : null}
                {knowledgeBases?.copyError || knowledgeBases?.loadError ? (
                  <div className={moduleClassNames("form-error")}>
                    {knowledgeBases.copyError || knowledgeBases.loadError}
                  </div>
                ) : null}
              </div>
              <div className={moduleClassNames("hub-workspace-block knowledge-base-overview")}>
                {knowledgeBases?.loading ? (
                  <div className={moduleClassNames("workspace-empty")}>{t("resourcesKnowledgeBasesLoading")}</div>
                ) : !knowledgeBases?.selected ? (
                  <div className={moduleClassNames("empty-state shell-empty-state hub-empty-state")}>
                    <BookOpen size={28} strokeWidth={1.5} aria-hidden="true" />
                    <strong>{t("resourcesKnowledgeBasesEmpty")}</strong>
                    <span>{t("resourcesKnowledgeBasesEmptyHint")}</span>
                    {knowledgeBases?.loginRequired ? (
                      <Button variant="primary" size="sm" onClick={() => void onKnowledgeBaseLogin?.()}>
                        {t("resourcesKnowledgeBasesLogin")}
                      </Button>
                    ) : null}
                  </div>
                ) : knowledgeBases.selected.configuredMCPName ? (
                  <div className={moduleClassNames("knowledge-base-configured")}>
                    <p>{t("resourcesKnowledgeBaseConfiguredDescription")}</p>
                    <div className={moduleClassNames("knowledge-base-mcp-name")}>
                      <span>{t("resourcesKnowledgeBaseMCPNameLabel")}</span>
                      <strong>{knowledgeBases.selected.configuredMCPName}</strong>
                    </div>
                    {knowledgeBaseMCPPreview ? (
                      <pre
                        className={moduleClassNames("knowledge-base-mcp-preview")}
                        aria-label={t("resourcesKnowledgeBaseMCPConfigLabel")}
                      >
                        <code>{knowledgeBaseMCPPreview}</code>
                      </pre>
                    ) : (
                      <div className={moduleClassNames("knowledge-base-mcp-loading")}>{t("resourcesMCPLoading")}</div>
                    )}
                  </div>
                ) : (
                  <div className={moduleClassNames("knowledge-base-help")}>
                    <strong>{t("resourcesKnowledgeBaseHowToTitle")}</strong>
                    <p>{t("resourcesKnowledgeBaseHowToDescription")}</p>
                  </div>
                )}
              </div>
            </>
          ) : activeResourceType === "template" && selectedTemplate ? (
            <>
              <div className={moduleClassNames("hub-inspector-hero")}>
                <div className={moduleClassNames("hub-inspector-hero-row")}>
                  <div className={moduleClassNames("hub-inspector-brand")}>
                    <div className={moduleClassNames("hub-inspector-copy")}>
                      <div className={moduleClassNames("hub-inspector-title-row")}>
                        <span className={moduleClassNames("hub-inspector-title-icon")} aria-hidden="true">
                          <ModelsIcon />
                        </span>
                        <h2>{hubTemplateFullName(selectedTemplate)}</h2>
                        <div className={moduleClassNames("hub-inspector-badge-row")}>
                          <span className={moduleClassNames("mini-badge template-runtime-badge")}>
                            {selectedTemplate.runtime_kind || selectedTemplate.workspace?.kind
                              ? formatRuntimeKindLabel(
                                  selectedTemplate.runtime_kind || selectedTemplate.workspace?.kind,
                                  t,
                                )
                              : "-"}
                          </span>
                          <span className={moduleClassNames("mini-badge template-source-badge")}>
                            <span className={moduleClassNames("template-source-badge-dot")} aria-hidden="true"></span>
                            {localizeTemplateSourceTag(selectedTemplate.source?.name, locale)}
                          </span>
                        </div>
                      </div>
                      <p>{selectedTemplate.description || selectedTemplate.id}</p>
                    </div>
                  </div>
                  <div className={moduleClassNames("hub-template-actions")}>
                    <Button variant="primary" size="md" onClick={() => onCreateFromTemplate?.(selectedTemplate)}>
                      <span>{t("createAgent")}</span>
                    </Button>
                    {canPublishTemplate ? (
                      <Button
                        variant="secondaryGray"
                        size="md"
                        loading={publishBusy}
                        disabled={publishBusy || publishDisabled}
                        title={publishDisabled ? t("agentPublishLoginRequired") : undefined}
                        onClick={() => {
                          setPublishTemplateIncludeMemory(false);
                          setPublishChoiceDialogOpen(true);
                        }}
                      >
                        {t("agentPublishCommunity")}
                      </Button>
                    ) : null}
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
                {templateReview ? (
                  <div className={moduleClassNames(`hub-template-review-alert ${templateReview.kind}`)} role="status">
                    <strong>
                      {templateReview.kind === "pending"
                        ? t("resourcesTemplateReviewPending")
                        : t("resourcesTemplateReviewFailed")}
                    </strong>
                    {templateReview.kind === "exception" && templateReview.paths.length ? (
                      <>
                        <span>{t("resourcesTemplateReviewFailedFiles")}</span>
                        <ul>
                          {templateReview.paths.map((path, index) => (
                            <li key={`${index}-${path}`}>{path}</li>
                          ))}
                        </ul>
                      </>
                    ) : null}
                  </div>
                ) : null}
              </div>

              <nav
                className={moduleClassNames("hub-template-section-nav")}
                aria-label={t("agentProfileSectionNavLabel")}
              >
                {templateTabs.map((tab) => {
                  const active = tab.id === activeTemplateTab;
                  return (
                    <button
                      key={tab.id}
                      type="button"
                      className={moduleClassNames("hub-template-section-tab", active && "active")}
                      aria-current={active ? "location" : undefined}
                      onClick={() => selectTemplateTab(tab.id)}
                    >
                      <span>{tab.label}</span>
                      {typeof tab.count === "number" ? (
                        <span
                          className={moduleClassNames("hub-template-section-tab-count")}
                          aria-label={String(tab.count)}
                        >
                          {tab.count}
                        </span>
                      ) : null}
                    </button>
                  );
                })}
              </nav>

              {activeTemplateTab === "profile" ? (
                <div className={moduleClassNames("profile-editor-shell hub-template-profile-shell")}>
                  <section className={moduleClassNames("profile-section")}>
                    <div className={moduleClassNames("profile-section-heading")}>
                      <div className={moduleClassNames("profile-section-title")}>{t("profileRuntimeSection")}</div>
                      <p className={moduleClassNames("profile-section-description")}>
                        {t("profileRuntimeSectionDescription")}
                      </p>
                    </div>
                    <div className={moduleClassNames("profile-grid-compact hub-template-profile-grid")}>
                      <label className={moduleClassNames("field")}>
                        <span>{t("resourcesRuntimeLabel")}</span>
                        <input
                          value={
                            selectedTemplate.runtime_kind
                              ? formatRuntimeKindLabel(selectedTemplate.runtime_kind, t)
                              : "-"
                          }
                          readOnly
                          disabled
                        />
                      </label>
                      <label className={moduleClassNames("field")}>
                        <span>{t("resourcesSourceLabel")}</span>
                        <input
                          value={localizeTemplateSourceTag(selectedTemplate.source?.name, locale)}
                          readOnly
                          disabled
                        />
                      </label>
                      <label className={moduleClassNames("field span-2")}>
                        <span>{t("resourcesImageLabel")}</span>
                        <input value={selectedTemplate.image || "-"} readOnly disabled />
                      </label>
                      <label className={moduleClassNames("field")}>
                        <span>{t("resourcesUpdatedAtLabel")}</span>
                        <input value={formatHubDateTime(selectedTemplate.updated_at, locale)} readOnly disabled />
                      </label>
                      <div className={moduleClassNames("field span-2 hub-template-env-field")}>
                        <div className={moduleClassNames("hub-template-env-heading")}>
                          <span>{t("resourcesTemplateEnvLabel")}</span>
                          <span className={moduleClassNames("hub-template-env-count")}>
                            {t("resourcesTemplateEnvCount", { count: templateImageEnv.length })}
                          </span>
                        </div>
                        {templateImageEnv.length ? (
                          <div className={moduleClassNames("hub-template-env-list")} role="list">
                            {templateImageEnv.map((item) => (
                              <div
                                className={moduleClassNames("hub-template-env-chip")}
                                role="listitem"
                                key={item.name}
                              >
                                <code className={moduleClassNames("hub-template-env-name")}>{item.name}</code>
                                <span
                                  className={moduleClassNames(
                                    `hub-template-env-status ${item.required ? "required" : "optional"}`,
                                  )}
                                >
                                  <span
                                    className={moduleClassNames("hub-template-env-status-dot")}
                                    aria-hidden="true"
                                  ></span>
                                  {item.required
                                    ? t("resourcesTemplateEnvRequiredBadge")
                                    : t("resourcesTemplateEnvOptional")}
                                </span>
                              </div>
                            ))}
                          </div>
                        ) : (
                          <div className={moduleClassNames("hub-template-env-empty")}>
                            {t("resourcesTemplateEnvNotRequired")}
                          </div>
                        )}
                      </div>
                    </div>
                  </section>
                </div>
              ) : activeTemplateTab === "instructions" ? (
                <section className={moduleClassNames("profile-section hub-template-instructions-section")}>
                  <div className={moduleClassNames("hub-template-instructions-header")}>
                    <div className={moduleClassNames("profile-section-heading")}>
                      <div className={moduleClassNames("profile-section-title")}>{t("agentInstructions")}</div>
                      <p className={moduleClassNames("profile-section-description")}>
                        {templateInstructionsMode === "default"
                          ? t("resourcesTemplateInstructionsDefaultHint")
                          : t("resourcesTemplateInstructionsAdvancedHint")}
                      </p>
                    </div>
                    <div
                      className={moduleClassNames("agent-instructions-mode-switch")}
                      role="group"
                      aria-label={t("agentInstructionsViewMode")}
                    >
                      <button
                        type="button"
                        aria-pressed={templateInstructionsMode === "default"}
                        onClick={() => setTemplateInstructionsMode("default")}
                      >
                        {t("agentInstructionsDefaultMode")}
                      </button>
                      <button
                        type="button"
                        aria-pressed={templateInstructionsMode === "advanced"}
                        onClick={() => setTemplateInstructionsMode("advanced")}
                      >
                        {t("agentInstructionsAdvancedMode")}
                      </button>
                    </div>
                  </div>
                  <div className={moduleClassNames("hub-template-instructions-content")}>
                    {templateInstructionsReadonly ? (
                      templateInstructionsValue.trim() ? (
                        <div className={moduleClassNames("hub-template-instructions-preview")}>
                          <div className={moduleClassNames("hub-template-instructions-preview-bar")}>
                            <FileCode2 size={16} strokeWidth={2} aria-hidden="true" />
                            <span>
                              {templateInstructionsMode === "advanced"
                                ? t("agentInstructionsEffective")
                                : t("agentInstructionsDefaultMode")}
                            </span>
                          </div>
                          <pre aria-label={t("agentInstructions")}>{templateInstructionsValue}</pre>
                        </div>
                      ) : (
                        <div className={moduleClassNames("hub-template-instructions-empty")}>
                          <span className={moduleClassNames("hub-template-instructions-empty-icon")} aria-hidden="true">
                            <FileCode2 size={20} strokeWidth={1.8} />
                          </span>
                          <strong>{t("resourcesTemplateInstructionsEmptyTitle")}</strong>
                          <p>{t("resourcesTemplateInstructionsEmptyDescription")}</p>
                          <Button
                            variant="secondaryGray"
                            size="sm"
                            onClick={() => setTemplateInstructionsMode("advanced")}
                          >
                            {t("resourcesTemplateInstructionsViewAdvancedAction")}
                          </Button>
                        </div>
                      )
                    ) : (
                      <div className={moduleClassNames("profile-grid-compact")}>
                        <label className={moduleClassNames("field span-2")}>
                          <span className={moduleClassNames("sr-only")}>{t("agentInstructions")}</span>
                          <textarea
                            className={moduleClassNames(
                              `compact-textarea hub-template-instructions-editor ${templateInstructionsMode === "advanced" ? "is-advanced" : "is-default"}`,
                            )}
                            value={templateInstructionsValue}
                            onInput={(event) => {
                              const value = event.currentTarget.value;
                              setTemplateInstructionsDraft((current) => {
                                if (templateInstructionsMode === "advanced") return value;
                                return replaceManagedAgentInstructions(current, value);
                              });
                            }}
                            placeholder={t("agentInstructionsPlaceholder")}
                          />
                        </label>
                      </div>
                    )}
                  </div>
                  {selectedTemplate.source?.kind === "local" ? (
                    <div className={moduleClassNames("form-actions")}>
                      <Button
                        variant="secondaryGray"
                        loading={templateInstructionsSaving}
                        onClick={saveTemplateInstructions}
                      >
                        {t("save")}
                      </Button>
                    </div>
                  ) : null}
                </section>
              ) : activeTemplateTab === "memory" ? (
                <section className="profile-section hub-template-memory-panel">
                  <div className="hub-template-memory-heading">
                    <div className="profile-section-heading">
                      <div className="profile-section-title">memory_summary.md</div>
                      <p className="profile-section-description">{t("resourcesTemplateMemoryDescription")}</p>
                      <code className="hub-template-memory-location">memories/memory_summary.md</code>
                    </div>
                  </div>
                  {workspaceFileError ? <div className="form-error">{workspaceFileError}</div> : null}
                  {workspaceFileLoading ? (
                    <div className="hub-template-memory-empty" role="status">
                      <strong>{t("resourcesWorkspaceFileLoading")}</strong>
                    </div>
                  ) : templateMemoryFile && !templateMemoryFile.binary ? (
                    <div className="agent-section-form hub-template-memory-document-shell">
                      <textarea
                        className="compact-textarea hub-template-memory-document"
                        value={templateMemoryFile.content || ""}
                        readOnly
                        aria-label={t("agentMemoryDocumentLabel")}
                      />
                    </div>
                  ) : (
                    <div className="hub-template-memory-empty">
                      <strong>{t("resourcesTemplateMemoryEmptyTitle")}</strong>
                      <p>{t("resourcesTemplateMemoryEmptyDescription")}</p>
                    </div>
                  )}
                </section>
              ) : activeTemplateTab === "skills" ? (
                <section
                  className={moduleClassNames("profile-section hub-template-summary-panel hub-template-skills-panel")}
                >
                  <div className={moduleClassNames("hub-template-summary-heading")}>
                    <div className={moduleClassNames("profile-section-heading")}>
                      <div className={moduleClassNames("profile-section-title")}>{t("agentSkillsTitle")}</div>
                      <p className={moduleClassNames("profile-section-description")}>
                        {t("resourcesTemplateSkillsDescription")}
                      </p>
                    </div>
                    <span className={moduleClassNames("hub-template-summary-count")}>
                      {t("resourcesTemplateSkillsCount", { count: templateSkills.length })}
                    </span>
                  </div>
                  {templateSkills.length ? (
                    <div className={moduleClassNames("hub-template-skills-list")}>
                      {templateSkills.map((skill) => (
                        <article className={moduleClassNames("hub-template-skill-row")} key={skill.name}>
                          <span className={moduleClassNames("hub-template-skill-icon")} aria-hidden="true">
                            <FileCode2 size={18} strokeWidth={1.8} />
                          </span>
                          <div className={moduleClassNames("hub-template-skill-copy")}>
                            <div className={moduleClassNames("hub-template-skill-name")}>{skill.name}</div>
                            <p>{skill.description || "-"}</p>
                          </div>
                        </article>
                      ))}
                    </div>
                  ) : (
                    <div className={moduleClassNames("hub-template-skills-empty")}>
                      <span className={moduleClassNames("hub-template-skill-icon")} aria-hidden="true">
                        <FileCode2 size={18} strokeWidth={1.8} />
                      </span>
                      <div>
                        <strong>{t("resourcesSkillsEmpty")}</strong>
                        <p>{t("resourcesTemplateSkillsEmptyHint")}</p>
                      </div>
                    </div>
                  )}
                </section>
              ) : activeTemplateTab === "mcp" ? (
                <section
                  className={moduleClassNames("profile-section hub-template-summary-panel hub-template-mcp-panel")}
                >
                  <div className={moduleClassNames("hub-template-summary-heading")}>
                    <div className={moduleClassNames("profile-section-heading")}>
                      <div className={moduleClassNames("profile-section-title")}>
                        {t("resourcesTemplateMCPServersTitle")}
                      </div>
                      <p className={moduleClassNames("profile-section-description")}>
                        {t("resourcesTemplateMCPServersDescription")}
                      </p>
                    </div>
                    <span className={moduleClassNames("hub-template-summary-count")}>
                      {t("resourcesTemplateMCPServersCount", { count: templateMCPServers.length })}
                    </span>
                  </div>
                  {templateMCPServers.length ? (
                    <div className={moduleClassNames("hub-template-mcp-list")}>
                      {templateMCPServers.map((server) => (
                        <article className={moduleClassNames("hub-template-mcp-row")} key={server.name}>
                          <span className={moduleClassNames("hub-template-mcp-icon")} aria-hidden="true">
                            <Server size={18} strokeWidth={1.8} />
                          </span>
                          <div className={moduleClassNames("hub-template-mcp-copy")}>
                            <div className={moduleClassNames("hub-template-mcp-name")}>{server.name}</div>
                            <p>{server.description || mcpServerDescription(server.config) || "-"}</p>
                          </div>
                        </article>
                      ))}
                    </div>
                  ) : workspaceFileLoading ? (
                    <div className={moduleClassNames("hub-template-mcp-empty")}>
                      <span className={moduleClassNames("hub-template-mcp-icon")} aria-hidden="true">
                        <Server size={18} strokeWidth={1.8} />
                      </span>
                      <div>
                        <strong>{t("resourcesWorkspaceFileLoading")}</strong>
                      </div>
                    </div>
                  ) : (
                    <div className={moduleClassNames("hub-template-mcp-empty")}>
                      <span className={moduleClassNames("hub-template-mcp-icon")} aria-hidden="true">
                        <Server size={18} strokeWidth={1.8} />
                      </span>
                      <div>
                        <strong>{t("resourcesMCPEmpty")}</strong>
                        <p>{t("resourcesTemplateMCPServersEmptyHint")}</p>
                      </div>
                    </div>
                  )}
                </section>
              ) : (
                <div className={moduleClassNames("hub-workspace-block hub-template-tab-panel")}>
                  <div className={moduleClassNames("hub-workspace-panels")}>
                    <WorkspaceFileTree
                      key={`${selectedTemplateId}-${activeTemplateTab}`}
                      className={moduleClassNames("hub-workspace-tree")}
                      entries={templateSectionWorkspaceEntries}
                      loading={workspaceTreeLoading}
                      loadingText={t("resourcesWorkspaceLoading")}
                      emptyText={t("resourcesWorkspacePreviewHint")}
                      selectedPath={templateSectionPath(
                        selectedWorkspacePath,
                        templateSection as "instructions" | "memories" | "skills" | "mcps",
                      )}
                      onSelectFile={(path) => onSelectWorkspaceFile(`${templateSection}/${path}`)}
                    />
                    <WorkspaceFilePreview
                      className={moduleClassNames("hub-workspace-preview")}
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
              )}
            </>
          ) : activeResourceType === "skill" && selectedSkill ? (
            <>
              <div className={moduleClassNames("hub-inspector-hero")}>
                <div className={moduleClassNames("hub-inspector-hero-row")}>
                  <div className={moduleClassNames("hub-inspector-brand")}>
                    <div className={moduleClassNames("hub-inspector-copy")}>
                      <div className={moduleClassNames("hub-inspector-title-row")}>
                        <span className={moduleClassNames("hub-inspector-title-icon")} aria-hidden="true">
                          <FileCode2 size={18} strokeWidth={2} />
                        </span>
                        <h2>{selectedSkill.name}</h2>
                      </div>
                      <p>{selectedSkill.description || selectedSkill.name}</p>
                    </div>
                  </div>
                  {canDeleteSkill ? (
                    <div className={moduleClassNames("hub-template-actions")}>
                      <Button
                        className={moduleClassNames("hub-skill-delete-button")}
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

              <div className={moduleClassNames("hub-workspace-block")}>
                <div className={moduleClassNames("hub-workspace-panels")}>
                  <WorkspaceFileTree
                    className={moduleClassNames("hub-workspace-tree")}
                    entries={skillEntries}
                    loading={skillTreeLoading}
                    loadingText={t("resourcesSkillFilesLoading")}
                    emptyText={skillTreeError || t("resourcesSkillFilesEmpty")}
                    selectedPath={selectedSkillPath}
                    onSelectFile={onSelectSkillFile}
                  />
                  <WorkspaceFilePreview
                    className={moduleClassNames("hub-workspace-preview")}
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
              <div className={moduleClassNames("hub-inspector-hero")}>
                <div className={moduleClassNames("hub-inspector-hero-row")}>
                  <div className={moduleClassNames("hub-inspector-brand")}>
                    <div className={moduleClassNames("hub-inspector-copy")}>
                      <div className={moduleClassNames("hub-inspector-title-row")}>
                        <span className={moduleClassNames("hub-inspector-title-icon")} aria-hidden="true">
                          <Server size={18} strokeWidth={2} />
                        </span>
                        <h2>{selectedMCPServer.name}</h2>
                        {selectedManagedMCPSource ? (
                          <span className={moduleClassNames("mini-badge mcp-knowledge-badge")}>
                            {t("resourcesKnowledgeMCPBadge")}
                          </span>
                        ) : null}
                      </div>
                      <p>
                        {selectedMCPServer.description ||
                          mcpServerDescription(selectedMCPServer.config) ||
                          selectedMCPServer.name}
                      </p>
                    </div>
                  </div>
                  <div className={moduleClassNames("hub-template-actions")}>
                    <Button
                      variant="secondaryGray"
                      size="md"
                      loading={mcpProbeBusy}
                      disabled={mcpMutationBusy}
                      onClick={handleProbeMCPDetail}
                    >
                      <RefreshCw size={16} strokeWidth={2} aria-hidden="true" />
                      {mcpProbeBusy ? t("resourcesMCPTesting") : t("resourcesMCPTest")}
                    </Button>
                    <Button
                      variant="primary"
                      size="md"
                      loading={mcpMutationBusy}
                      disabled={mcpProbeBusy}
                      onClick={handleSaveMCPDetail}
                    >
                      {mcpMutationBusy ? t("resourcesMCPSaving") : t("resourcesMCPSave")}
                    </Button>
                    <Button
                      variant="outlineDanger"
                      size="md"
                      disabled={mcpMutationBusy || mcpProbeBusy}
                      onClick={() => setMCPDeleteDialogOpen(true)}
                    >
                      <Trash2 size={16} strokeWidth={2} />
                      <span>{t("resourcesMCPDelete")}</span>
                    </Button>
                  </div>
                </div>
              </div>

              {mcpStateError || mcpMutationError || mcpProbeError ? (
                <div className={moduleClassNames("form-error")} aria-live="polite">
                  {mcpStateError || mcpMutationError || mcpProbeError}
                </div>
              ) : null}
              {mcpStateLoading ? (
                <div className={moduleClassNames("workspace-empty")}>{t("resourcesMCPLoading")}</div>
              ) : null}

              {selectedManagedMCPSource && (mcpSourceError || mcpSourceStatus?.updateAvailable) ? (
                <div
                  className={moduleClassNames(
                    "mcp-source-notice",
                    mcpSourceStatus?.updateAvailable ? "update-available" : "",
                    mcpSourceError && !mcpSourceStatus?.updateAvailable ? "check-failed" : "",
                  )}
                  role="status"
                >
                  <div className={moduleClassNames("mcp-source-notice-copy")}>
                    <strong>
                      {mcpSourceStatus?.updateAvailable
                        ? t("resourcesKnowledgeMCPUpdateAvailable")
                        : t("resourcesKnowledgeMCPCheckFailed")}
                    </strong>
                    <span>
                      {mcpSourceStatus?.updateAvailable
                        ? t("resourcesKnowledgeMCPUpdateHint")
                        : t("resourcesKnowledgeMCPCheckFailedHint")}
                    </span>
                  </div>
                  <div className={moduleClassNames("mcp-source-notice-actions")}>
                    {mcpSourceStatus?.updateAvailable ? (
                      <Button
                        variant="primary"
                        size="sm"
                        loading={mcpSourceSyncBusy}
                        disabled={mcpSourceBusy}
                        onClick={handleSyncMCPSource}
                      >
                        {t("resourcesMCPSourceUpdate")}
                      </Button>
                    ) : (
                      <Button
                        variant="secondaryGray"
                        size="sm"
                        loading={mcpSourceBusy}
                        disabled={mcpSourceSyncBusy}
                        onClick={handleCheckMCPSource}
                      >
                        {t("resourcesMCPSourceRetry")}
                      </Button>
                    )}
                  </div>
                </div>
              ) : null}

              <div className={moduleClassNames("hub-workspace-block mcp-server-document-block")}>
                <JSONConfigEditor
                  label={t("resourcesMCPServerDocumentLabel")}
                  value={mcpDetailDocument}
                  onChange={handleMCPDetailDocumentChange}
                  invalid={Boolean(mcpDetailError)}
                  minRows={12}
                />
                {mcpDetailError ? (
                  <div className={moduleClassNames("form-error hub-json-editor-error")}>{mcpDetailError}</div>
                ) : null}
                {mcpProbeResult ? <MCPProbePanel result={mcpProbeResult} t={t} /> : null}
              </div>
            </>
          ) : (
            <div className={moduleClassNames("empty-state shell-empty-state hub-empty-state")}>
              <span className={moduleClassNames("rich-empty-mark")} aria-hidden="true">
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
        <DialogContent className={moduleClassNames("hub-skill-delete-dialog")}>
          <DialogHeader className={moduleClassNames("hub-skill-delete-dialog-header")}>
            <div className={moduleClassNames("hub-skill-delete-dialog-copy")}>
              <DialogTitle>{t("resourcesDeleteSkill")}</DialogTitle>
              <DialogDescription>
                {t("resourcesDeleteSkillConfirmMessage", { name: selectedSkill?.name || "" })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <DialogFooter className={moduleClassNames("hub-skill-delete-dialog-actions")}>
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
      <DialogRoot open={publishSuccessDialogOpen} onOpenChange={setPublishSuccessDialogOpen}>
        <DialogContent className={moduleClassNames("hub-skill-delete-dialog")}>
          <DialogHeader className={moduleClassNames("hub-skill-delete-dialog-header")}>
            <div className={moduleClassNames("hub-skill-delete-dialog-copy")}>
              <DialogTitle>
                {publishPartialMessage
                  ? t("resourcesPublishCommunityDeployFailedTitle")
                  : t("resourcesPublishCommunitySuccessTitle")}
              </DialogTitle>
              <DialogDescription className={moduleClassNames(publishPartialMessage && "hub-publish-partial-message")}>
                {publishPartialMessage ? publishPartialMessage : t("resourcesPublishCommunitySuccessMessage")}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <DialogFooter className={moduleClassNames("hub-skill-delete-dialog-actions")}>
            <Button variant="primary" size="sm" onClick={() => setPublishSuccessDialogOpen(false)}>
              {t("resourcesPublishCommunitySuccessDismiss")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot
        open={publishChoiceDialogOpen}
        onOpenChange={(open) => {
          setPublishChoiceDialogOpen(open);
          if (!open) {
            setPublishTemplateIncludeMemory(false);
          }
        }}
      >
        <DialogContent className={moduleClassNames("agent-publish-dialog")}>
          <DialogHeader>
            <div>
              <DialogTitle>{t("agentPublishTemplateTitle")}</DialogTitle>
              <DialogDescription>{t("agentPublishTemplateCommunitySubtitle")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {templateMemoryEnabled ? (
            <DialogBody>
              <label className={moduleClassNames("hub-template-publish-memory-option")}>
                <Checkbox
                  aria-label={t("agentPublishIncludeMemory")}
                  checked={publishTemplateIncludeMemory}
                  onCheckedChange={(checked) => setPublishTemplateIncludeMemory(checked === true)}
                />
                <span>
                  <strong>{t("agentPublishIncludeMemory")}</strong>
                  <small>{t("agentPublishIncludeMemoryWarning")}</small>
                </span>
              </label>
            </DialogBody>
          ) : null}
          {publishError ? <div className={moduleClassNames("form-error")}>{publishError}</div> : null}
          <DialogFooter>
            <Button
              variant="secondaryGray"
              size="md"
              disabled={publishBusy}
              onClick={() => setPublishChoiceDialogOpen(false)}
            >
              {t("cancel")}
            </Button>
            <Button
              variant="secondaryGray"
              size="md"
              loading={publishBusy}
              loadingLabel={t("agentPublishing")}
              disabled={publishBusy}
              onClick={() => void handlePublishTemplate(false)}
            >
              {t("agentPublishCommunityTemplateOnly")}
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={publishBusy}
              loadingLabel={t("agentPublishing")}
              disabled={publishBusy}
              onClick={() => void handlePublishTemplate(true)}
            >
              {t("agentPublishCommunityAndDeploy")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot
        open={Boolean(knowledgeBases?.pendingMCPKnowledgeBase)}
        onOpenChange={(open) => {
          if (!open && !knowledgeBases?.copyBusyID) {
            knowledgeBases?.cancelMCPConfig();
          }
        }}
      >
        <DialogContent className={moduleClassNames("knowledge-base-guide-dialog")}>
          <DialogHeader className={moduleClassNames("hub-skill-delete-dialog-header")}>
            <div className={moduleClassNames("hub-skill-delete-dialog-copy")}>
              <DialogTitle>{t("resourcesKnowledgeBaseGuideTitle")}</DialogTitle>
            </div>
            <DialogCloseButton
              label={t("close")}
              size="sm"
              variant="tertiaryGray"
              disabled={Boolean(knowledgeBases?.copyBusyID)}
            />
          </DialogHeader>
          <DialogBody>
            <DialogDescription className={moduleClassNames("knowledge-base-guide-description")}>
              {t("resourcesKnowledgeBaseGuideDescription", {
                name: knowledgeBases?.pendingMCPKnowledgeBase?.name || "",
              })}
            </DialogDescription>
            {knowledgeBases?.copyError ? (
              <div className={moduleClassNames("form-error knowledge-base-guide-error")}>
                {knowledgeBases.copyError}
              </div>
            ) : null}
          </DialogBody>
          <DialogFooter className={moduleClassNames("hub-skill-delete-dialog-actions")}>
            <Button
              variant="secondaryGray"
              size="sm"
              disabled={Boolean(knowledgeBases?.copyBusyID)}
              onClick={() => knowledgeBases?.cancelMCPConfig()}
            >
              {t("resourcesKnowledgeBaseGuideLater")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              loading={knowledgeBases?.copyBusyID === knowledgeBases?.pendingMCPKnowledgeBase?.id}
              onClick={() => void knowledgeBases?.confirmMCPConfig()}
            >
              {t("resourcesKnowledgeBaseGuideContinue")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot
        open={mcpCreateDialogOpen}
        onOpenChange={(open) => {
          if (open) {
            setMCPDraftDocument(mcpCreateInitialDocument || DEFAULT_MCP_SERVER_DOCUMENT);
            setMCPFormError("");
            setMCPCreateMode("manual");
            onMCPCreateDialogOpenChange?.(true);
          } else {
            closeMCPFormDialog();
          }
        }}
      >
        <DialogContent className={moduleClassNames("mcp-dialog")}>
          <DialogHeader className={moduleClassNames("hub-skill-delete-dialog-header")}>
            <div className={moduleClassNames("hub-skill-delete-dialog-copy")}>
              <DialogTitle>{t("resourcesMCPCreateTitle")}</DialogTitle>
              <DialogDescription>{t("resourcesMCPFormDescription")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <DialogBody className={moduleClassNames("mcp-form")}>
            <div className={moduleClassNames("mcp-form-mode")} role="tablist" aria-label={t("resourcesMCPCreateTitle")}>
              <Button
                active={mcpCreateMode === "manual"}
                aria-selected={mcpCreateMode === "manual"}
                role="tab"
                size="sm"
                variant={mcpCreateMode === "manual" ? "primary" : "secondaryGray"}
                onClick={() => setMCPCreateMode("manual")}
              >
                <Server size={15} strokeWidth={2} aria-hidden="true" />
                {t("resourcesMCPManualTab")}
              </Button>
              <Button
                active={mcpCreateMode === "remote"}
                aria-selected={mcpCreateMode === "remote"}
                role="tab"
                size="sm"
                variant={mcpCreateMode === "remote" ? "primary" : "secondaryGray"}
                onClick={() => setMCPCreateMode("remote")}
              >
                <CloudDownload size={15} strokeWidth={2} aria-hidden="true" />
                {t("resourcesMCPRemoteInstallTab")}
              </Button>
            </div>
            {mcpCreateMode === "manual" ? (
              <>
                <JSONConfigEditor
                  hideLabel
                  label={t("resourcesMCPServerDocumentJSONLabel")}
                  value={mcpDraftDocument}
                  onChange={handleMCPDraftDocumentChange}
                  invalid={Boolean(mcpFormError)}
                  minRows={12}
                />
                {mcpFormError || mcpCreateError ? (
                  <div className={moduleClassNames("form-error hub-json-editor-error")}>
                    {mcpFormError || mcpCreateError}
                  </div>
                ) : null}
              </>
            ) : (
              <RemoteMCPList
                error={remoteMCPServersError || mcpMutationError}
                hasMore={remoteMCPServersHasMore}
                installedServers={mcpServers}
                installBusy={remoteMCPInstallBusy}
                items={remoteMCPServers}
                loading={remoteMCPServersLoading}
                loadingMore={remoteMCPServersLoadingMore}
                onInstall={onInstallRemoteMCP}
                onLoadMore={onLoadMoreRemoteMCPServers}
                onRefresh={onRefreshRemoteMCPServers}
                onSearchChange={onRemoteMCPServersSearchChange}
                onVisibleChange={onRemoteMCPVisibleChange}
                search={remoteMCPServersSearch}
                t={t}
              />
            )}
          </DialogBody>
          <DialogFooter className={moduleClassNames("hub-skill-delete-dialog-actions")}>
            <Button variant="secondaryGray" size="sm" disabled={mcpMutationBusy} onClick={closeMCPFormDialog}>
              {t("cancel")}
            </Button>
            {mcpCreateMode === "manual" ? (
              <Button variant="primary" size="sm" loading={mcpMutationBusy} onClick={handleSaveMCP}>
                {mcpMutationBusy ? t("resourcesMCPSaving") : t("resourcesMCPSave")}
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot open={knowledgeBaseDeleteDialogOpen} onOpenChange={setKnowledgeBaseDeleteDialogOpen}>
        <DialogContent className={moduleClassNames("hub-skill-delete-dialog")}>
          <DialogHeader className={moduleClassNames("hub-skill-delete-dialog-header")}>
            <div className={moduleClassNames("hub-skill-delete-dialog-copy")}>
              <DialogTitle>{t("resourcesKnowledgeBaseRemoveMCP")}</DialogTitle>
              <DialogDescription>
                {t("resourcesKnowledgeBaseRemoveMCPConfirm", {
                  name: configuredKnowledgeBaseMCP?.name || knowledgeBases?.selected?.configuredMCPName || "",
                })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {mcpMutationError ? <div className={moduleClassNames("form-error")}>{mcpMutationError}</div> : null}
          <DialogFooter className={moduleClassNames("hub-skill-delete-dialog-actions")}>
            <Button
              variant="secondaryGray"
              size="sm"
              disabled={mcpMutationBusy}
              onClick={() => setKnowledgeBaseDeleteDialogOpen(false)}
            >
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              size="sm"
              loading={mcpMutationBusy}
              disabled={!configuredKnowledgeBaseMCP}
              onClick={handleDeleteKnowledgeBaseMCPConfirm}
            >
              {t("resourcesKnowledgeBaseRemoveMCP")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <DialogRoot open={mcpDeleteDialogOpen} onOpenChange={setMCPDeleteDialogOpen}>
        <DialogContent className={moduleClassNames("hub-skill-delete-dialog")}>
          <DialogHeader className={moduleClassNames("hub-skill-delete-dialog-header")}>
            <div className={moduleClassNames("hub-skill-delete-dialog-copy")}>
              <DialogTitle>{t("resourcesMCPDelete")}</DialogTitle>
              <DialogDescription>
                {t("resourcesMCPDeleteConfirmMessage", { name: selectedMCPServer?.name || "" })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {mcpMutationError ? <div className={moduleClassNames("form-error")}>{mcpMutationError}</div> : null}
          <DialogFooter className={moduleClassNames("hub-skill-delete-dialog-actions")}>
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
