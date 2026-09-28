import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchAgentSkills, fetchAgentSkillsFile } from "@/api/agents";
import { localizeAPIError } from "@/shared/i18n";
import type { TranslateFn } from "@/models/conversations";
import type { MCPServer } from "@/models/mcp";
import type { SlashSkillOption } from "@/models/slashCommands";
import { mcpServerDisplayName, mcpServerDetailConfig } from "@/models/mcp";
import { WorkspaceFileTree, WorkspaceFilePreview } from "@/components/business/WorkspaceFileTree";
import {
  Button,
  Switch,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogCloseButton,
  DialogBody,
  DialogFooter,
} from "@/components/ui";
import styles from "./AgentResourceDetails.module.css";

type Props = {
  agentID: string;
  skill?: SlashSkillOption;
  server?: MCPServer;
  t: TranslateFn;
  busy: boolean;
  error?: string;
  onRetry?: () => Promise<void>;
  canToggle: boolean;
  portalContainer?: HTMLElement | null;
  onClose: () => void;
  onToggle: () => void;
  onDelete: () => void;
};

export function AgentResourceDetails({
  agentID,
  skill,
  server,
  t,
  busy,
  error,
  onRetry,
  canToggle,
  portalContainer,
  onClose,
  onToggle,
  onDelete,
}: Props) {
  const name = skill?.name || server?.name || "";
  const [path, setPath] = useState(`${name}/SKILL.md`);
  const tree = useQuery({
    queryKey: ["agent-skill-tree", agentID, name],
    queryFn: () => fetchAgentSkills(agentID, name),
    enabled: Boolean(skill),
  });
  const file = useQuery({
    queryKey: ["agent-skill-file", agentID, path],
    queryFn: () => fetchAgentSkillsFile(agentID, path),
    enabled: Boolean(skill),
  });
  const enabled = skill ? skill.enabled !== false : server?.config?.enabled !== false;
  const dialogClassName = skill ? `${styles.dialog} hub-standard-skill-dialog` : styles.dialog;
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className={dialogClassName} portalContainer={portalContainer}>
        <DialogHeader>
          <div>
            <DialogTitle>{server ? mcpServerDisplayName(server) : name}</DialogTitle>
            <DialogDescription>{skill?.description || server?.description || name}</DialogDescription>
          </div>
          <div className="flex shrink-0 items-center gap-4">
            <Switch aria-label={name} checked={enabled} disabled={busy || !canToggle} onCheckedChange={onToggle} />
            <DialogCloseButton label={t("close")} />
          </div>
        </DialogHeader>
        <DialogBody className={skill ? "hub-standard-skill-dialog-body" : undefined}>
          {error ? (
            <div role="alert" className="form-error">
              {error}
              {onRetry ? (
                <Button disabled={busy} onClick={() => void onRetry()}>
                  {t("retry")}
                </Button>
              ) : null}
            </div>
          ) : null}
          {skill ? (
            <div className="hub-workspace-panels hub-skill-detail-file-panels">
              <WorkspaceFileTree
                className="hub-workspace-tree"
                entries={tree.data?.entries ?? []}
                loading={tree.isFetching}
                selectedPath={path}
                onSelectFile={setPath}
                loadingText={t("resourcesSkillFilesLoading")}
                emptyText={
                  tree.error
                    ? localizeAPIError(tree.error, t, t("agentSkillsLoadFailed"))
                    : t("resourcesSkillPreviewHint")
                }
              />
              <WorkspaceFilePreview
                className="hub-workspace-preview"
                portalContainer={portalContainer}
                file={file.data}
                loading={file.isFetching}
                error={file.error ? localizeAPIError(file.error, t, t("agentSkillsLoadFailed")) : ""}
                loadingText={t("resourcesWorkspaceFileLoading")}
                emptyTitle={t("resourcesSkillPreviewTitle")}
                emptyHint={t("resourcesSkillPreviewHint")}
                emptyIcon={<AgentSkillPreviewEmptyIcon />}
                binaryText={t("resourcesWorkspaceBinary")}
                emptyFileText={t("resourcesWorkspaceEmptyFile")}
                previewText={t("workspacePreviewPreviewTab")}
                codeText={t("workspacePreviewCodeTab")}
                viewToggleLabel={t("workspacePreviewViewMode")}
                closeText={t("close")}
                truncatedText={t("workspacePreviewTruncated")}
              />
            </div>
          ) : server ? (
            <pre className={styles.config}>{JSON.stringify(mcpServerDetailConfig(server.config), null, 2)}</pre>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="outlineDanger" disabled={busy} onClick={onDelete}>
            {t(skill ? "agentDeleteSkill" : "agentDeleteMCP")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

function AgentSkillPreviewEmptyIcon() {
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
