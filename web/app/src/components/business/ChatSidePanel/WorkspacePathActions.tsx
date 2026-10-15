import styles from "./WorkspacePathActions.module.css";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FolderOpen, ChevronDown } from "lucide-react";
import {
  Button,
  DropdownMenuRoot,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui";
import { fetchWorkspaceLocation, openWorkspaceLocation, type WorkspaceLocation } from "@/api/chatFiles";
import { writeTextToClipboard } from "@/shared/lib/clipboard";
import type { TranslateFn } from "@/models/conversations";

export function WorkspacePathActions({
  agentID,
  roomID,
  path,
  location,
  active = true,
  t,
}: {
  agentID: string;
  roomID: string;
  path: string;
  location?: WorkspaceLocation;
  active?: boolean;
  t: TranslateFn;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const query = useQuery({
    queryKey: ["chat-workspace-location", roomID, agentID, path],
    queryFn: ({ signal }) => fetchWorkspaceLocation(agentID, roomID, path, signal),
    enabled: !location && active,
    retry: false,
  });
  const current = location ?? query.data;
  const targets = (current?.open_targets ?? []).filter((target) => target !== "notepad" || !current?.directory);
  const labels: Record<string, string> = {
    system: t("chatOpenSystem"),
    terminal: t("chatOpenTerminal"),
    vscode: "Visual Studio Code",
    sublime: "Sublime Text",
    iterm: "iTerm2",
    notepad: "Notepad",
  };
  async function open(target: string) {
    setBusy(true);
    setError("");
    try {
      await openWorkspaceLocation(agentID, roomID, path, target);
    } catch {
      setError(t("chatOpenFailed"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className={styles.actions}>
      <DropdownMenuRoot
        onOpenChange={(opened) => {
          if (opened && !location) void query.refetch();
        }}
      >
        <DropdownMenuTrigger asChild>
          <Button
            style={{ width: 40, minWidth: 40 }}
            iconOnly
            size="sm"
            variant="tertiaryGray"
            aria-label={t("chatOpenPath")}
            disabled={busy}
            title={t("chatOpenPath")}
          >
            <span className={styles.icons}>
              <FolderOpen size={16} />
              <ChevronDown size={12} />
            </span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          {targets.map((target) => (
            <DropdownMenuItem key={target} onSelect={() => void open(target)}>
              {labels[target] ?? target}
            </DropdownMenuItem>
          ))}
          {targets.length ? <DropdownMenuSeparator /> : null}
          <DropdownMenuItem
            disabled={!current?.absolute_path}
            onSelect={() => {
              void writeTextToClipboard(current?.absolute_path ?? "").then((ok) =>
                setError(ok ? "" : t("chatCopyPathFailed")),
              );
            }}
          >
            {t("chatCopyPath")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenuRoot>
      {error ? (
        <span className={styles.error} role="alert">
          {error}
        </span>
      ) : query.error ? (
        <span className={styles.error} role="alert">
          {t("attachmentPreviewFailed")}
        </span>
      ) : null}
    </div>
  );
}
