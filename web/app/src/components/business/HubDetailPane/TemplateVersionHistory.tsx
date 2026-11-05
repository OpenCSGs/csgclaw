import { useRef, useState } from "react";
import { ChevronRight, FileText, Folder, History, LoaderCircle, MoreHorizontal, RotateCcw, Trash2 } from "lucide-react";
import {
  Breadcrumb,
  Button,
  EmptyState,
  DropdownMenuRoot,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogBody,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui";
import type { HubTemplate } from "@/models/hubWorkspace";
import { formatHubDateTime } from "@/models/hubWorkspace";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import { useTemplateVersions } from "./useTemplateVersions";
import styles from "./TemplateVersionHistory.module.css";

type Props = { template: HubTemplate; t: TranslateFn; locale: LocaleCode; onRestored?: (item: HubTemplate) => unknown };
type VersionAction = { kind: "restore" | "delete"; revision: number };

export function TemplateVersionHistory(props: Props) {
  return <VersionContent key={props.template.id} {...props} />;
}

function VersionContent({ template, t, locale, onRestored }: Props) {
  const [action, setAction] = useState<VersionAction | null>(null);
  const actionTrigger = useRef<HTMLElement | null>(null);
  const versionButtons = useRef(new Map<number, HTMLButtonElement>());
  const menuTriggers = useRef(new Map<number, HTMLButtonElement>());
  const state = useTemplateVersions(template, t, onRestored);
  const activeRevision = template.revision || 1;
  const latestRevision = Math.max(template.latest_revision || activeRevision, ...state.versions.map((v) => v.revision));
  const selected = state.versions.find((v) => v.revision === state.revision);
  const path = state.file?.path || state.listing?.path || "";
  const segments = path.split("/").filter(Boolean);
  const openAction = (next: VersionAction, trigger: HTMLElement | null) => {
    actionTrigger.current = trigger;
    setAction(next);
  };
  const confirmAction = async () => {
    if (!action) return;
    const success =
      action.kind === "restore" ? await state.rollback(action.revision) : await state.remove(action.revision);
    if (success) setAction(null);
  };
  return (
    <div className={styles.content} aria-busy={state.busy}>
      <div className={styles.toolbar}>
        <p className={styles.hint}>{t("templateVersionHistoryHint")}</p>
        {template.uid ? (
          <p className={styles.identity}>
            <span>{t("templateVersionUID")}</span>
            <code>{template.uid}</code>
          </p>
        ) : null}
      </div>
      {state.error && !action ? (
        <p className={styles.error} role="alert">
          {state.error}
        </p>
      ) : null}
      {!state.busy && !state.versions.length && !state.error ? (
        <EmptyState title={t("templateVersionsEmpty")} icon={<History size={24} />} />
      ) : null}
      {state.versions.length ? (
        <div className={styles.layout}>
          <div className={styles.versionRail}>
            <h3 className={styles.sectionLabel}>
              {t("templateVersionHistory")} <span>{state.versions.length}</span>
            </h3>
            <ul className={styles.versions} aria-label={t("templateVersionHistory")}>
              {state.versions.map((version) => (
                <li
                  key={version.revision}
                  className={styles.versionRow}
                  data-selected={state.revision === version.revision}
                >
                  <Button
                    ref={(node) => {
                      if (node) versionButtons.current.set(version.revision, node);
                      else versionButtons.current.delete(version.revision);
                    }}
                    className={styles.versionButton}
                    variant="tertiaryGray"
                    size="sm"
                    aria-label={`V${version.revision}${version.revision === activeRevision ? ` · ${t("templateVersionActive")}` : ""}`}
                    aria-current={state.revision === version.revision ? "true" : undefined}
                    disabled={state.busy}
                    onClick={() => {
                      if (state.revision !== version.revision || state.error) void state.browse(version.revision);
                    }}
                  >
                    <span className={styles.versionInfo}>
                      <span className={styles.versionHeading}>
                        <strong>V{version.revision}</strong>
                        {version.revision === activeRevision ? (
                          <span className={styles.activeBadge}>{t("templateVersionActive")}</span>
                        ) : null}
                        {version.revision === latestRevision ? (
                          <span className={styles.latest}>{t("templateVersionLatest")}</span>
                        ) : null}
                      </span>
                      <span
                        className={styles.commitMessage}
                        data-empty={!version.message}
                        title={version.message || undefined}
                      >
                        {version.message || t("templateVersionNoMessage")}
                      </span>
                      <time
                        className={styles.metadata}
                        dateTime={version.created_at}
                        title={formatHubDateTime(version.created_at, locale)}
                      >
                        {formatHubDateTime(version.created_at, locale)}
                      </time>
                      {version.author ? (
                        <span className={styles.metadata}>{version.author.name || version.author.id}</span>
                      ) : null}
                    </span>
                  </Button>
                  {version.revision !== activeRevision ? (
                    <DropdownMenuRoot>
                      <DropdownMenuTrigger asChild>
                        <Button
                          ref={(node) => {
                            if (node) menuTriggers.current.set(version.revision, node);
                            else menuTriggers.current.delete(version.revision);
                          }}
                          className={styles.moreButton}
                          variant="tertiaryGray"
                          size="sm"
                          disabled={state.busy}
                          aria-label={t("templateVersionMore", { version: String(version.revision) })}
                        >
                          <MoreHorizontal size={16} aria-hidden="true" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent size="sm">
                        <DropdownMenuItem
                          danger
                          onSelect={() =>
                            openAction(
                              { kind: "delete", revision: version.revision },
                              menuTriggers.current.get(version.revision) || null,
                            )
                          }
                        >
                          <Trash2 size={14} aria-hidden="true" />
                          {t("templateVersionDelete")} V{version.revision}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenuRoot>
                  ) : null}
                </li>
              ))}
            </ul>
          </div>
          <section
            className={styles.detail}
            aria-label={
              selected
                ? t("templateVersionPreview", { version: String(selected.revision) })
                : t("templateVersionContent")
            }
          >
            {selected ? (
              <>
                <header className={styles.detailHeader}>
                  <div className={styles.detailTitle}>
                    <h3>
                      V{selected.revision} · {t("templateVersionContent")}
                    </h3>
                    <span className={styles.readonly}>{t("templateVersionReadonly")}</span>
                  </div>
                  {selected.revision !== activeRevision ? (
                    <Button
                      variant="secondaryGray"
                      size="sm"
                      disabled={state.busy}
                      onClick={(event) =>
                        openAction({ kind: "restore", revision: selected.revision }, event.currentTarget)
                      }
                    >
                      <RotateCcw size={14} aria-hidden="true" />
                      {t("templateVersionSetActive")}
                    </Button>
                  ) : null}
                </header>
                <dl className={styles.metadataGrid}>
                  <dt>{t("agentName")}</dt>
                  <dd>{selected.name || "—"}</dd>
                  <dt>{t("resourcesRuntimeLabel")}</dt>
                  <dd>{selected.runtime_kind || "—"}</dd>
                  <dt>{t("agentDescription")}</dt>
                  <dd>{selected.description || "—"}</dd>
                </dl>
                <div className={styles.browser}>
                  <div className={styles.browserToolbar}>
                    <Breadcrumb
                      aria-label={t("templateVersionPath")}
                      overflowLabel={t("templateVersionPathMore")}
                      items={[
                        {
                          label: t("templateVersionRoot"),
                          current: segments.length === 0,
                          onClick: () => {
                            if (!state.busy) void state.browse(selected.revision);
                          },
                        },
                        ...segments.map((segment, index) => ({
                          label: segment,
                          current: index === segments.length - 1,
                          onClick: () => {
                            if (!state.busy)
                              void state.browse(selected.revision, segments.slice(0, index + 1).join("/"));
                          },
                        })),
                      ]}
                    />
                    {state.busy ? (
                      <LoaderCircle size={15} className={styles.spinner} aria-label={t("templateVersionBusy")} />
                    ) : null}
                  </div>
                  {state.file ? (
                    <div className={styles.filePreview}>
                      <div className={styles.fileHeading}>
                        <FileText size={14} aria-hidden="true" />
                        <span>{state.file.path.split("/").pop()}</span>
                        {state.file.truncated ? (
                          <span className={styles.metadata}>{t("workspacePreviewTruncated")}</span>
                        ) : null}
                      </div>
                      <pre className={styles.preview}>
                        <code>
                          {state.file.binary
                            ? t("templateVersionBinary")
                            : state.file.content || t("resourcesWorkspaceEmptyFile")}
                        </code>
                      </pre>
                    </div>
                  ) : (
                    <div className={styles.files}>
                      {state.listing?.entries?.map((entry) => (
                        <Button
                          className={styles.fileButton}
                          key={entry.path}
                          size="sm"
                          variant="tertiaryGray"
                          disabled={state.busy}
                          onClick={() => void state.browse(selected.revision, entry.path, entry.type !== "dir")}
                        >
                          <span className={styles.fileRow}>
                            {entry.type === "dir" ? (
                              <Folder size={16} className={styles.folderIcon} aria-hidden="true" />
                            ) : (
                              <FileText size={16} aria-hidden="true" />
                            )}
                            <span className={styles.fileName}>{entry.name}</span>
                            {entry.type === "dir" ? <ChevronRight size={14} aria-hidden="true" /> : null}
                          </span>
                        </Button>
                      ))}
                      {!state.busy && state.listing && !state.listing.entries?.length ? (
                        <p className={styles.emptyDirectory}>{t("templateVersionDirectoryEmpty")}</p>
                      ) : null}
                    </div>
                  )}
                </div>
              </>
            ) : (
              <EmptyState
                title={t("templateVersionSelect")}
                description={t("templateVersionSelectHint")}
                icon={<History size={24} />}
              />
            )}
          </section>
        </div>
      ) : null}
      {state.busy && !state.versions.length ? (
        <p className={styles.loading} role="status">
          <LoaderCircle size={15} className={styles.spinner} aria-hidden="true" />
          {t("templateVersionBusy")}
        </p>
      ) : null}
      <DialogRoot
        open={action !== null}
        onOpenChange={(open) => {
          if (!open && !state.busy) setAction(null);
        }}
      >
        {action ? (
          <DialogContent
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              const target = actionTrigger.current?.isConnected
                ? actionTrigger.current
                : versionButtons.current.get(activeRevision);
              target?.focus();
            }}
          >
            <DialogHeader>
              <div className={styles.confirmationCopy}>
                <DialogTitle>
                  {t(action.kind === "restore" ? "templateVersionSetActive" : "templateVersionDelete")} V
                  {action.revision}
                </DialogTitle>
                <DialogDescription>
                  {action.kind === "restore"
                    ? t("templateVersionSwitchSummary", { from: String(activeRevision), to: String(action.revision) })
                    : t("templateVersionDeleteHint", { version: String(action.revision) })}
                </DialogDescription>
              </div>
            </DialogHeader>
            {action.kind === "restore" || state.error ? (
              <DialogBody>
                {action.kind === "restore" ? <p className={styles.hint}>{t("templateVersionSaveHint")}</p> : null}
                {state.error ? (
                  <p className={styles.error} role="alert">
                    {state.error}
                  </p>
                ) : null}
              </DialogBody>
            ) : null}
            <DialogFooter>
              <Button variant="secondaryGray" size="sm" disabled={state.busy} onClick={() => setAction(null)}>
                {t("cancel")}
              </Button>
              <Button
                variant={action.kind === "restore" ? "primary" : "danger"}
                size="sm"
                loading={state.busy}
                onClick={() => void confirmAction()}
              >
                {action.kind === "restore"
                  ? t("templateVersionRollbackConfirm", { version: String(action.revision) })
                  : `${t("templateVersionDelete")} V${action.revision}`}
              </Button>
            </DialogFooter>
          </DialogContent>
        ) : null}
      </DialogRoot>
    </div>
  );
}
