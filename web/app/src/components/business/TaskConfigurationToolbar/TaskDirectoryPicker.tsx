import { useState } from "react";
import { Folder, FolderPlus, House } from "lucide-react";
import { Button, Combobox } from "@/components/ui";
import { pickLocalDirectoryPath } from "@/components/business/ProfileControls/runtimeOptionDirectoryPicker";
import { readTaskDirectories, rememberTaskDirectory } from "@/shared/storage/taskDirectories";
import type { TranslateFn } from "@/models/conversations";
import styles from "./TaskConfigurationToolbar.module.css";

function directoryName(path: string): string {
  return (
    path
      .replace(/[\\/]+$/, "")
      .split(/[\\/]/)
      .at(-1) || path
  );
}

export function TaskDirectoryPicker({
  value,
  defaultDirectory,
  locked,
  disabled,
  onChoose,
  t,
}: {
  value: string;
  defaultDirectory: string;
  locked: boolean;
  disabled: boolean;
  onChoose: (path: string) => Promise<void>;
  t: TranslateFn;
}) {
  const [paths, setPaths] = useState(readTaskDirectories);
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState(false);
  const [error, setError] = useState("");
  const effective = value || defaultDirectory;
  const label = (
    <>
      <Folder size={16} aria-hidden="true" />
      <span>{effective ? directoryName(effective) : t("iaWorkdirDefault")}</span>
    </>
  );
  async function choose(path: string) {
    setError("");
    try {
      await onChoose(path);
      if (path) setPaths(rememberTaskDirectory(path));
      setOpen(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("sendFailed"));
      setOpen(true);
    }
  }
  if (locked)
    return (
      <span
        className={styles.lockedDirectory}
        aria-label={t("iaWorkdirTitle")}
        title={`${effective || t("iaWorkdirDefaultHint")} · ${t("iaWorkdirLocked")}`}
      >
        {label}
      </span>
    );
  const directories = [...new Set([value, defaultDirectory, ...paths].filter(Boolean))];
  return (
    <Combobox
      value={value}
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        setError("");
        if (next) setPaths(readTaskDirectories());
      }}
      disabled={disabled || picking}
      size="sm"
      selectedLabel={<span className={styles.directoryLabel}>{label}</span>}
      triggerClassName={styles.directoryTrigger}
      triggerProps={{ "aria-label": t("iaWorkdirTitle"), title: effective || t("iaWorkdirDefaultHint") }}
      contentClassName={styles.directoryMenu}
      contentProps={{ side: "top", align: "start" }}
      searchPlaceholder={t("iaWorkdirSearch")}
      emptyLabel={t("iaWorkdirEmpty")}
      options={[
        {
          value: "",
          alwaysVisible: true,
          separatorAfter: true,
          textValue: t("iaWorkdirDefault"),
          label: (
            <span className={styles.directoryLabel}>
              <House size={18} />
              <span>
                {t("iaWorkdirDefault")}
                <small>{defaultDirectory || t("iaWorkdirDefaultHint")}</small>
              </span>
            </span>
          ),
        },
        ...directories.map((path) => ({
          value: path,
          group: t("iaWorkdirAdded"),
          textValue: path,
          label: (
            <span className={styles.directoryLabel}>
              <Folder size={18} />
              <span>
                {directoryName(path)}
                <small>{path}</small>
              </span>
            </span>
          ),
        })),
      ]}
      onValueChange={(path) => void choose(path)}
      footer={
        <div className={styles.menuFooter}>
          {error ? (
            <p role="alert" className={styles.error}>
              {error}
            </p>
          ) : null}
          <Button
            variant="tertiaryGray"
            size="sm"
            className={styles.configureButton}
            disabled={disabled || picking}
            onClick={async () => {
              setPicking(true);
              setError("");
              try {
                const path = await pickLocalDirectoryPath();
                if (path) await choose(path);
              } catch (cause) {
                setError(cause instanceof Error ? cause.message : t("sendFailed"));
              } finally {
                setPicking(false);
              }
            }}
          >
            <FolderPlus size={16} />
            {t(picking ? "iaWorkdirAdding" : "iaWorkdirAdd")}
          </Button>
        </div>
      }
    />
  );
}
