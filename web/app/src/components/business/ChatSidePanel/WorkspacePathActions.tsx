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
import { fetchFileLocation, openFileLocation, type WorkspaceLocation } from "@/api/chatFiles";
import { writeTextToClipboard } from "@/shared/lib/clipboard";
import type { ComposerFileReference } from "@/models/fileReferences";
import type { TranslateFn } from "@/models/conversations";

export function WorkspacePathActions({
  roomID,
  reference,
  token = "",
  location,
  active = true,
  t,
}: {
  roomID: string;
  reference: ComposerFileReference;
  token?: string;
  location?: WorkspaceLocation;
  active?: boolean;
  t: TranslateFn;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const query = useQuery({
    queryKey: ["chat-file-location", roomID, reference, token],
    queryFn: ({ signal }) => fetchFileLocation(roomID, reference, token, signal),
    enabled: false,
    retry: false,
  });
  const current = location ?? query.data;
  const targets = current?.open_targets ?? [];
  const labels: Record<string, string> = {
    system: t("chatOpenSystem"),
    terminal: t("chatOpenTerminal"),
  };
  async function open(target: string) {
    setBusy(true);
    setError("");
    try {
      await openFileLocation(roomID, reference, target);
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
          if (opened && active && !location) void query.refetch();
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
        <DropdownMenuContent className={styles.menu}>
          {query.isFetching && !location ? (
            <DropdownMenuItem disabled>{t("chatWorkspaceLocationLoading")}</DropdownMenuItem>
          ) : query.isError && !location ? (
            <DropdownMenuItem disabled>{t("chatWorkspaceLocationFailed")}</DropdownMenuItem>
          ) : null}
          {targets.map((target) => (
            <DropdownMenuItem key={target.id} onSelect={() => void open(target.id)}>
              {labels[target.id] ?? target.label ?? target.id}
              {target.default ? ` (${t("chatOpenDefault")})` : ""}
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
      ) : null}
    </div>
  );
}
