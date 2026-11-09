import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { errorMessage } from "@/api/client";
import { fetchSkillFile, fetchSkillTree } from "@/api/skills";
import { WorkspaceFilePreview, WorkspaceFileTree } from "@/components/business/WorkspaceFileTree";
import { Button } from "@/components/ui";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import type { TranslateFn } from "@/models/conversations";
import type { SkillSummary } from "@/models/skillhub";
import styles from "./SkillDetailFiles.module.css";

export function SkillDetailFiles({
  skill,
  market = false,
  t,
}: {
  skill: SkillSummary;
  market?: boolean;
  t: TranslateFn;
}) {
  const remote = market ? skill : undefined;
  const root = remote ? "" : skill.name;
  const sourceKey = skill.discovery?.key;
  const [selectedPath, setSelectedPath] = useState(root ? `${root}/SKILL.md` : "SKILL.md");
  const tree = useQuery({
    // Keep remote previews inside the existing login-environment cache reset scope.
    queryKey: remote
      ? [...workspaceQueryKeys.officialSkills(), "tree", remote.remotePath, remote.remoteRef]
      : [...workspaceQueryKeys.skillTree(root), ...(sourceKey ? ["source", sourceKey] : [])],
    queryFn: () => fetchSkillTree(root, remote),
    retry: false,
  });
  const file = useQuery({
    queryKey: remote
      ? [...workspaceQueryKeys.officialSkills(), "file", remote.remotePath, remote.remoteRef, selectedPath]
      : [...workspaceQueryKeys.skillFile(selectedPath), ...(sourceKey ? ["source", sourceKey] : [])],
    queryFn: () => fetchSkillFile(selectedPath, remote),
    retry: false,
  });
  const error = tree.error || file.error;

  return (
    <>
      {error ? (
        <div className="flex items-center gap-3" role="alert">
          <span className="form-error">{errorMessage(error, t("resourcesSkillFileLoadFailed"))}</span>
          <Button
            size="sm"
            variant="secondaryGray"
            disabled={tree.isFetching || file.isFetching}
            onClick={() => {
              if (tree.isError) void tree.refetch();
              if (file.isError) void file.refetch();
            }}
          >
            {t("retry")}
          </Button>
        </div>
      ) : null}
      <div className={styles.panels}>
        <WorkspaceFileTree
          entries={tree.data?.entries ?? []}
          loading={tree.isFetching}
          loadingText={t("resourcesSkillFilesLoading")}
          emptyText={t("resourcesSkillPreviewHint")}
          selectedPath={selectedPath}
          onSelectFile={setSelectedPath}
        />
        <WorkspaceFilePreview
          file={file.data ?? null}
          loading={file.isFetching}
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
    </>
  );
}
