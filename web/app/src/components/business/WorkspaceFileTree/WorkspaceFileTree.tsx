import { useEffect, useMemo, useState } from "react";
import { ChevronRight, FileText, Folder, LoaderCircle } from "lucide-react";
import {
  buildInitialCollapsedWorkspaceDirs,
  buildVisibleWorkspaceEntries,
  workspaceAncestorDirs,
} from "@/models/workspace";
import type { CollapsedWorkspaceDirs, WorkspaceEntry, WorkspaceTreeDepthStyle } from "@/models/workspace";
import "./WorkspaceFileTree.css";

type WorkspaceFileTreeProps = {
  className?: string;
  emptyText: string;
  entries: readonly WorkspaceEntry[] | null | undefined;
  loading?: boolean;
  loadingText: string;
  selectedPath?: string;
  loadingPaths?: ReadonlySet<string>;
  onSelectFile?: (path: string) => void;
  onToggleDir?: (path: string) => void | Promise<void>;
};

export function WorkspaceFileTree({
  className = "",
  emptyText,
  entries,
  loading = false,
  loadingText,
  selectedPath = "",
  loadingPaths,
  onSelectFile,
  onToggleDir,
}: WorkspaceFileTreeProps) {
  const [collapsedDirs, setCollapsedDirs] = useState<CollapsedWorkspaceDirs>({});
  const visibleEntries = useMemo(() => buildVisibleWorkspaceEntries(entries, collapsedDirs), [entries, collapsedDirs]);

  useEffect(() => {
    const initial = buildInitialCollapsedWorkspaceDirs(entries);
    setCollapsedDirs((current) =>
      Object.fromEntries(
        Object.keys(initial).map((path) => [path, Object.hasOwn(current, path) ? current[path] : true]),
      ),
    );
  }, [entries]);

  useEffect(() => {
    if (!selectedPath) {
      return;
    }
    const ancestors = workspaceAncestorDirs(selectedPath);
    if (!ancestors.length) {
      return;
    }
    setCollapsedDirs((current) => {
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
  }, [selectedPath]);

  function toggleDir(path: string) {
    if (collapsedDirs[path]) {
      void onToggleDir?.(path);
    }
    setCollapsedDirs((current) => ({
      ...current,
      [path]: !current[path],
    }));
  }

  return (
    <div className={`workspace-file-tree ${className}`.trim()}>
      {loading ? (
        <div className="workspace-empty">{loadingText}</div>
      ) : visibleEntries.length === 0 ? (
        <div className="workspace-empty">{emptyText}</div>
      ) : (
        visibleEntries.map((entry) => {
          const isDir = entry.type === "dir";
          const collapsed = isDir && Boolean(collapsedDirs[entry.path]);
          const dirLoading = isDir && loadingPaths?.has(entry.path);
          return (
            <button
              key={entry.path}
              type="button"
              className={`workspace-tree-row ${entry.type} ${isDir ? "toggleable" : ""} ${
                !isDir && selectedPath === entry.path ? "active" : ""
              }`.trim()}
              style={{ "--workspace-tree-depth": entry.depth ?? 0 } as WorkspaceTreeDepthStyle}
              onClick={() => (isDir ? toggleDir(entry.path) : onSelectFile?.(entry.path))}
              aria-expanded={isDir ? !collapsed : undefined}
            >
              <span className={`workspace-tree-toggle ${!isDir ? "spacer" : ""}`.trim()} aria-hidden="true">
                {dirLoading ? (
                  <LoaderCircle className="animate-spin" size={14} strokeWidth={2} />
                ) : isDir ? (
                  <ChevronRight className={collapsed ? "collapsed" : ""} size={14} strokeWidth={2} />
                ) : null}
              </span>
              <span className="workspace-tree-glyph" aria-hidden="true">
                {isDir ? <Folder size={16} strokeWidth={2} /> : <FileText size={16} strokeWidth={2} />}
              </span>
              <span className="workspace-tree-label">{entry.name || entry.path}</span>
            </button>
          );
        })
      )}
    </div>
  );
}
