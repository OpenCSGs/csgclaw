import { useState } from "react";
import { Folder, House } from "lucide-react";
import {
  Button,
  Combobox,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
  TextInput,
} from "@/components/ui";
import { pickLocalDirectoryPath } from "@/components/business/ProfileControls/runtimeOptionDirectoryPicker";
import { readTaskDirectories, rememberTaskDirectory } from "@/shared/storage/taskDirectories";
import type { TranslateFn } from "@/models/conversations";
import styles from "./TaskConfigurationToolbar.module.css";

function AddDirectoryIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M13 7L11.8845 4.76892C11.5634 4.1268 11.4029 3.80573 11.1634 3.57116C10.9516 3.36373 10.6963 3.20597 10.4161 3.10931C10.0992 3 9.74021 3 9.02229 3H5.2C4.0799 3 3.51984 3 3.09202 3.21799C2.71569 3.40973 2.40973 3.71569 2.21799 4.09202C2 4.51984 2 5.0799 2 6.2V7M2 7H17.2C18.8802 7 19.7202 7 20.362 7.32698C20.9265 7.6146 21.3854 8.07354 21.673 8.63803C22 9.27976 22 10.1198 22 11.8V16.2C22 17.8802 22 18.7202 21.673 19.362C21.3854 19.9265 20.9265 20.3854 20.362 20.673C19.7202 21 18.8802 21 17.2 21H6.8C5.11984 21 4.27976 21 3.63803 20.673C3.07354 20.3854 2.6146 19.9265 2.32698 19.362C2 18.7202 2 17.8802 2 16.2V7ZM12 17V11M9 14H15"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

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
  defaultLabel,
  defaultHint,
  locked,
  disabled,
  onChoose,
  t,
}: {
  value: string;
  defaultDirectory: string;
  defaultLabel?: string;
  defaultHint?: string;
  locked: boolean;
  disabled: boolean;
  onChoose: (path: string) => Promise<void>;
  t: TranslateFn;
}) {
  const [paths, setPaths] = useState(readTaskDirectories);
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState(false);
  const [error, setError] = useState("");
  const [manualOpen, setManualOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const [applying, setApplying] = useState(false);
  const effective = value || defaultDirectory;
  const automaticLabel = defaultLabel ?? t("iaWorkdirDefault");
  const automaticHint = defaultHint ?? t("iaWorkdirDefaultHint");
  const label = (
    <>
      <Folder size={16} aria-hidden="true" />
      <span>{effective ? directoryName(effective) : automaticLabel}</span>
    </>
  );
  async function choose(path: string) {
    setError("");
    try {
      await onChoose(path);
      if (path) setPaths(rememberTaskDirectory(path));
      setOpen(false);
      return true;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("sendFailed"));
      if (!manualOpen) setOpen(true);
      return false;
    }
  }
  if (locked)
    return (
      <span
        className={styles.lockedDirectory}
        aria-label={t("iaWorkdirTitle")}
        title={`${effective || automaticHint} · ${t("iaWorkdirLocked")}`}
      >
        {label}
      </span>
    );
  const directories = [...new Set([value, defaultDirectory, ...paths].filter(Boolean))];
  return (
    <>
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
        triggerProps={{ "aria-label": t("iaWorkdirTitle"), title: effective || automaticHint }}
        contentClassName={styles.directoryMenu}
        contentProps={{ side: "top", align: "start" }}
        searchPlaceholder={t("iaWorkdirSearch")}
        emptyLabel={t("iaWorkdirEmpty")}
        options={[
          {
            value: "",
            alwaysVisible: true,
            separatorAfter: true,
            textValue: automaticLabel,
            label: (
              <span className={styles.directoryLabel}>
                <House size={18} />
                <span>
                  {automaticLabel}
                  <small>{defaultDirectory || automaticHint}</small>
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
              onClick={() => {
                setDraft(effective);
                setError("");
                setOpen(false);
                setManualOpen(true);
              }}
            >
              <AddDirectoryIcon />
              {t("iaWorkdirManual")}
            </Button>
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
              <AddDirectoryIcon />
              {t(picking ? "iaWorkdirAdding" : "iaWorkdirAdd")}
            </Button>
          </div>
        }
      />
      <DialogRoot
        open={manualOpen}
        onOpenChange={(next) => {
          if (!applying) {
            setManualOpen(next);
            setError("");
          }
        }}
      >
        <DialogContent>
          <form
            onSubmit={async (event) => {
              event.preventDefault();
              if (!draft.trim() || applying || disabled) return;
              setApplying(true);
              try {
                if (await choose(draft.trim())) setManualOpen(false);
              } finally {
                setApplying(false);
              }
            }}
          >
            <DialogHeader>
              <DialogTitle>{t("taskConfigurationDirectoryPath")}</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <TextInput
                aria-label={t("taskConfigurationDirectoryPath")}
                placeholder="/path/to/workspace"
                value={draft}
                disabled={applying}
                onChange={(event) => setDraft(event.target.value)}
              />
              {error ? (
                <p role="alert" className={styles.error}>
                  {error}
                </p>
              ) : null}
            </DialogBody>
            <DialogFooter>
              <Button type="submit" disabled={disabled || applying || !draft.trim()}>
                {t("taskConfigurationApply")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </DialogRoot>
    </>
  );
}
