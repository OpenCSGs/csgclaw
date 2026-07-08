import { useCallback, useEffect, useRef, useState } from "react";
import type { ChangeEvent, DragEvent, UIEvent } from "react";
import { CloudDownload, FileCode2, RefreshCw, UploadCloud } from "lucide-react";
import {
  Button,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  TextInput,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { hasSkillName, remoteSkillInstallName } from "@/models/skillhub";
import { classNames } from "@/shared/lib/classNames";
import styles from "./SkillUploadDialog.module.css";
import type { SkillSummary } from "@/models/skillhub";

type RemoteSkillInstallOptions = {
  replace?: boolean;
};

export type SkillUploadDialogProps = {
  busy: boolean;
  error: string;
  installedSkills: readonly SkillSummary[];
  onInstallRemoteSkill?: (skill: SkillSummary, options?: RemoteSkillInstallOptions) => Promise<unknown>;
  onLoadMoreRemoteSkills?: () => Promise<unknown>;
  onOpenChange: (open: boolean) => void;
  onRefreshRemoteSkills?: () => Promise<unknown>;
  onRemoteSkillsSearchChange?: (value: string) => void;
  onRemoteVisibleChange?: (visible: boolean) => void;
  onSubmit: (file: File) => Promise<unknown>;
  open: boolean;
  remoteInstallBusy: string;
  remoteInstallError: string;
  remoteSkills: readonly SkillSummary[];
  remoteSkillsError: string;
  remoteSkillsHasMore: boolean;
  remoteSkillsLoading: boolean;
  remoteSkillsLoadingMore: boolean;
  remoteSkillsSearch: string;
  t: TranslateFn;
};

type SkillUploadMode = "zip" | "remote";

export function SkillUploadDialog({
  open,
  onOpenChange,
  onInstallRemoteSkill,
  onLoadMoreRemoteSkills,
  onRefreshRemoteSkills,
  onRemoteSkillsSearchChange,
  onRemoteVisibleChange,
  onSubmit,
  busy,
  error,
  installedSkills,
  remoteInstallBusy,
  remoteInstallError,
  remoteSkillsHasMore,
  remoteSkills,
  remoteSkillsError,
  remoteSkillsLoading,
  remoteSkillsLoadingMore,
  remoteSkillsSearch,
  t,
}: SkillUploadDialogProps) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [localError, setLocalError] = useState("");
  const [dragOver, setDragOver] = useState(false);
  const [mode, setMode] = useState<SkillUploadMode>("zip");

  useEffect(() => {
    if (!open) {
      setSelectedFile(null);
      setLocalError("");
      setDragOver(false);
      setMode("zip");
    }
  }, [open]);

  useEffect(() => {
    onRemoteVisibleChange?.(open && mode === "remote");
  }, [mode, onRemoteVisibleChange, open]);

  const setFile = useCallback(
    (file: File | null | undefined) => {
      if (!file) {
        return;
      }
      if (
        !String(file.name || "")
          .toLowerCase()
          .endsWith(".zip")
      ) {
        setSelectedFile(null);
        setLocalError(t("resourcesSkillUploadDropHint"));
        return;
      }
      setSelectedFile(file);
      setLocalError("");
    },
    [t],
  );

  const handleFileChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      setFile(event.target.files?.[0] ?? null);
      event.target.value = "";
    },
    [setFile],
  );

  const handleDrop = useCallback(
    (event: DragEvent<HTMLButtonElement>) => {
      event.preventDefault();
      setDragOver(false);
      setFile(event.dataTransfer.files?.[0] ?? null);
    },
    [setFile],
  );

  const handleSubmit = useCallback(async () => {
    if (!selectedFile) {
      setLocalError(t("resourcesSkillUploadDropHint"));
      return;
    }
    const result = await onSubmit(selectedFile);
    if (result) {
      onOpenChange(false);
    }
  }, [onOpenChange, onSubmit, selectedFile, t]);

  const handleRemoteInstall = useCallback(
    async (skill: SkillSummary, options: RemoteSkillInstallOptions = {}) => {
      if (!onInstallRemoteSkill) {
        return;
      }
      const result = await onInstallRemoteSkill(skill, options);
      if (result) {
        onOpenChange(false);
      }
    },
    [onInstallRemoteSkill, onOpenChange],
  );

  const handleRemoteListScroll = useCallback(
    (event: UIEvent<HTMLDivElement>) => {
      if (!onLoadMoreRemoteSkills || remoteSkillsLoadingMore || !remoteSkillsHasMore) {
        return;
      }
      const target = event.currentTarget;
      const remaining = target.scrollHeight - target.scrollTop - target.clientHeight;
      if (remaining <= 80) {
        void onLoadMoreRemoteSkills();
      }
    },
    [onLoadMoreRemoteSkills, remoteSkillsHasMore, remoteSkillsLoadingMore],
  );

  return (
    <DialogRoot open={open} onOpenChange={onOpenChange}>
      <DialogContent className={styles.dialog}>
        <DialogHeader>
          <div>
            <DialogTitle>{t("resourcesSkillUpload")}</DialogTitle>
            <DialogDescription>{t("resourcesSkillUploadSubtitle")}</DialogDescription>
          </div>
        </DialogHeader>
        <DialogBody className={styles.body}>
          <div className={styles.mode} role="tablist" aria-label={t("resourcesSkillUpload")}>
            <Button
              active={mode === "zip"}
              aria-selected={mode === "zip"}
              role="tab"
              size="sm"
              variant={mode === "zip" ? "primary" : "secondaryGray"}
              onClick={() => setMode("zip")}
            >
              <UploadCloud size={15} strokeWidth={2} aria-hidden="true" />
              {t("resourcesSkillUploadZipTab")}
            </Button>
            <Button
              active={mode === "remote"}
              aria-selected={mode === "remote"}
              role="tab"
              size="sm"
              variant={mode === "remote" ? "primary" : "secondaryGray"}
              onClick={() => setMode("remote")}
            >
              <CloudDownload size={15} strokeWidth={2} aria-hidden="true" />
              {t("resourcesSkillRemoteInstallTab")}
            </Button>
          </div>
          {mode === "zip" ? (
            <>
              <button
                type="button"
                className={classNames(
                  styles.dropzone,
                  dragOver && styles.dragOver,
                  (localError || error) && styles.dropzoneError,
                )}
                onClick={() => inputRef.current?.click()}
                onDragEnter={(event) => {
                  event.preventDefault();
                  setDragOver(true);
                }}
                onDragLeave={(event) => {
                  event.preventDefault();
                  setDragOver(false);
                }}
                onDragOver={(event) => {
                  event.preventDefault();
                  setDragOver(true);
                }}
                onDrop={handleDrop}
              >
                <span className={styles.uploadIcon} aria-hidden="true">
                  <UploadCloud size={20} strokeWidth={1.8} />
                </span>
                <span className={styles.uploadCopy}>
                  <strong>{t("resourcesSkillUploadDropTitle")}</strong>
                  <small>{selectedFile ? selectedFile.name : t("resourcesSkillUploadDropHint")}</small>
                </span>
              </button>
              <input
                ref={inputRef}
                className={styles.fileInput}
                type="file"
                accept=".zip,application/zip"
                onChange={handleFileChange}
              />
              {localError || error ? <div className="form-error">{localError || error}</div> : null}
            </>
          ) : (
            <div className={styles.remotePanel} role="tabpanel">
              <label className={styles.remoteSearch}>
                <TextInput
                  type="search"
                  aria-label={t("resourcesSkillRemoteSearchPlaceholder")}
                  value={remoteSkillsSearch}
                  placeholder={t("resourcesSkillRemoteSearchPlaceholder")}
                  onChange={(event) => onRemoteSkillsSearchChange?.(event.currentTarget.value)}
                />
              </label>
              {remoteSkillsError ? (
                <div className={styles.remoteState}>
                  <span>{remoteSkillsError}</span>
                  {onRefreshRemoteSkills ? (
                    <Button size="sm" variant="secondaryGray" onClick={() => void onRefreshRemoteSkills()}>
                      <RefreshCw size={14} strokeWidth={2} aria-hidden="true" />
                      {t("resourcesSkillRemoteRefresh")}
                    </Button>
                  ) : null}
                </div>
              ) : remoteSkillsLoading && !remoteSkills.length ? (
                <div className={styles.remoteState}>{t("resourcesSkillRemoteSkillsLoading")}</div>
              ) : remoteSkills.length ? (
                <>
                  <div className={styles.remoteList} onScroll={handleRemoteListScroll}>
                    {remoteSkills.map((item) => {
                      const installKey = item.remotePath || item.name;
                      const installed = hasSkillName(installedSkills, remoteSkillInstallName(item));
                      const description = item.description || item.remotePath || item.name;
                      const rowContent = (
                        <>
                          <span className={styles.remoteIcon} aria-hidden="true">
                            <FileCode2 size={16} strokeWidth={2} />
                          </span>
                          <span className={styles.remoteMain}>
                            <span className={classNames(styles.remoteTitle, "truncate")}>{item.name}</span>
                            <span className={classNames(styles.remoteMeta, "truncate")}>{description}</span>
                          </span>
                        </>
                      );
                      return (
                        <div className={styles.remoteRow} key={installKey} title={description}>
                          {item.remoteURL ? (
                            <a
                              className={styles.remoteLink}
                              href={item.remoteURL}
                              target="_blank"
                              rel="noopener noreferrer"
                            >
                              {rowContent}
                            </a>
                          ) : (
                            <span className={styles.remoteLink}>{rowContent}</span>
                          )}
                          <Button
                            size="sm"
                            variant="primary"
                            loading={remoteInstallBusy === installKey}
                            disabled={!onInstallRemoteSkill || Boolean(remoteInstallBusy)}
                            onClick={() => void handleRemoteInstall(item, { replace: installed })}
                          >
                            {remoteInstallBusy === installKey
                              ? t("resourcesSkillRemoteInstalling")
                              : installed
                                ? t("resourcesSkillRemoteReplaceAction")
                                : t("resourcesSkillRemoteInstallAction")}
                          </Button>
                        </div>
                      );
                    })}
                    {remoteSkillsLoadingMore ? (
                      <div className={styles.remoteListState}>{t("resourcesSkillRemoteSkillsLoading")}</div>
                    ) : null}
                  </div>
                  {remoteInstallError ? <div className="form-error">{remoteInstallError}</div> : null}
                </>
              ) : (
                <div className={styles.remoteState}>{t("resourcesSkillRemoteSkillsEmpty")}</div>
              )}
            </div>
          )}
          <div className={styles.actions}>
            <Button variant="secondaryGray" size="md" onClick={() => onOpenChange(false)} disabled={busy}>
              {t("close")}
            </Button>
            {mode === "zip" ? (
              <Button variant="primary" size="md" onClick={() => void handleSubmit()} loading={busy} disabled={busy}>
                {busy ? t("resourcesSkillUploadSubmitting") : t("resourcesSkillUploadSubmit")}
              </Button>
            ) : null}
          </div>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}
