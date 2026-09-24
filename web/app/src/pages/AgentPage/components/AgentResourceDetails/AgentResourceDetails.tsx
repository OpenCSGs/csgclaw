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
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className={styles.dialog} portalContainer={portalContainer}>
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
        <DialogBody>
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
            <div className={styles.files}>
              <WorkspaceFileTree
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
                portalContainer={portalContainer}
                file={file.data}
                loading={file.isFetching}
                error={file.error ? localizeAPIError(file.error, t, t("agentSkillsLoadFailed")) : ""}
                loadingText={t("resourcesWorkspaceFileLoading")}
                emptyTitle={t("resourcesSkillPreviewTitle")}
                emptyHint={t("resourcesSkillPreviewHint")}
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
