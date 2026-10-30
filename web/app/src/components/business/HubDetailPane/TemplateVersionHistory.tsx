import { useState } from "react";
import { ChevronDown, ChevronRight, FileText, Folder, History, LoaderCircle, RotateCcw } from "lucide-react";
import { Breadcrumb, Button, EmptyState, Field, Select } from "@/components/ui";
import type { HubTemplate } from "@/models/hubWorkspace";
import { formatHubDateTime } from "@/models/hubWorkspace";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import { useTemplateVersions } from "./useTemplateVersions";
import styles from "./TemplateVersionHistory.module.css";

type Props = { template: HubTemplate; t: TranslateFn; locale: LocaleCode; onRestored?: (item: HubTemplate) => unknown };

export function TemplateVersionHistory(props: Props) {
  const [open, setOpen] = useState(false);
  return (
    <details className={styles.history} onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary className={styles.summary}>
        <span className={styles.summaryTitle}>
          <History size={16} aria-hidden="true" />
          {props.t("templateVersionHistory")}
        </span>
        <span className={styles.summaryEnd}>
          {props.template.revision ? (
            <span className={styles.badge}>
              {props.t("templateVersionActive")} V{props.template.revision}
            </span>
          ) : null}
          <ChevronDown size={16} className={styles.disclosure} aria-hidden="true" />
        </span>
      </summary>
      {open ? <VersionContent {...props} /> : null}
    </details>
  );
}

function VersionContent({ template, t, locale, onRestored }: Props) {
  const [rollbackOpen, setRollbackOpen] = useState(false);
  const [targetRevision, setTargetRevision] = useState("");
  const state = useTemplateVersions(template, t, onRestored);
  const latestRevision = Math.max(
    template.latest_revision || template.revision || 0,
    ...state.versions.map((version) => version.revision),
  );
  const rollbackOptions = state.versions.filter((version) => version.revision !== template.revision);
  const path = state.file?.path || state.listing?.path || "";
  const segments = path.split("/").filter(Boolean);
  const revision = state.revision;
  return (
    <div className={styles.content} aria-busy={state.busy}>
      <p className={styles.hint}>
        {t("templateVersionHistoryHint")} {t("templateVersionActive")} V{template.revision || "—"}
      </p>
      {state.error ? (
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
            <span className={styles.sectionLabel}>{t("templateVersionHistory")}</span>
            <ul className={styles.versions} aria-label={t("templateVersionHistory")}>
              {state.versions.map((version) => (
                <li key={version.revision}>
                  <Button
                    className={styles.versionButton}
                    variant={revision === version.revision ? "secondaryColor" : "tertiaryGray"}
                    size="sm"
                    aria-label={`V${version.revision}`}
                    aria-pressed={revision === version.revision}
                    disabled={state.busy}
                    onClick={() => void state.browse(version.revision)}
                  >
                    <span className={styles.versionInfo}>
                      <span className={styles.versionHeading}>
                        <strong>V{version.revision}</strong>
                        {version.revision === latestRevision ? (
                          <span className={styles.latest}>{t("templateVersionLatest")}</span>
                        ) : null}
                        {version.revision === template.revision ? (
                          <span className={styles.latest}>{t("templateVersionActive")}</span>
                        ) : null}
                      </span>
                      <span className={styles.commitMessage}>{version.message || t("templateVersionNoMessage")}</span>
                      {version.author ? (
                        <span className={styles.metadata}>{version.author.name || version.author.id}</span>
                      ) : null}
                      <time className={styles.metadata} dateTime={version.created_at}>
                        {formatHubDateTime(version.created_at, locale)}
                      </time>
                    </span>
                  </Button>
                </li>
              ))}
            </ul>
          </div>
          <div className={styles.detail}>
            {revision === null ? (
              <EmptyState
                title={t("templateVersionSelect")}
                description={t("templateVersionSelectHint")}
                icon={<History size={24} />}
              />
            ) : (
              <section aria-label={`V${revision}`}>
                <header className={styles.detailHeader}>
                  <div className={styles.detailTitle}>
                    <span className={styles.badge}>V{revision}</span>
                    <h4>{t("templateVersionContent")}</h4>
                  </div>
                  <span className={styles.readonly}>{t("templateVersionReadonly")}</span>
                </header>
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
                            if (!state.busy) void state.browse(revision);
                          },
                        },
                        ...segments.map((segment, index) => ({
                          label: segment,
                          current: index === segments.length - 1,
                          onClick: () => {
                            if (!state.busy) void state.browse(revision, segments.slice(0, index + 1).join("/"));
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
                          onClick={() => void state.browse(revision, entry.path, entry.type !== "dir")}
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
                      {!state.busy && !state.listing?.entries?.length ? (
                        <p className={styles.emptyDirectory}>{t("templateVersionDirectoryEmpty")}</p>
                      ) : null}
                    </div>
                  )}
                </div>
                {rollbackOptions.length > 0 ? (
                  <div className={styles.saveSection}>
                    <div className={styles.saveActions}>
                      <Button
                        variant="secondaryGray"
                        size="sm"
                        disabled={state.busy}
                        onClick={() => {
                          setTargetRevision(
                            String(
                              rollbackOptions.find((version) => version.revision === revision)?.revision ||
                                rollbackOptions[0].revision,
                            ),
                          );
                          setRollbackOpen(!rollbackOpen);
                        }}
                      >
                        <RotateCcw size={14} aria-hidden="true" />
                        {t("templateRestoreVersion")}
                      </Button>
                    </div>
                    {rollbackOpen ? (
                      <>
                        <p className={styles.hint}>{t("templateVersionSaveHint")}</p>
                        <Field label={t("templateVersionRollbackTarget")}>
                          <Select
                            value={targetRevision}
                            onValueChange={setTargetRevision}
                            disabled={state.busy}
                            options={rollbackOptions.map((version) => ({
                              value: String(version.revision),
                              label: `V${version.revision}`,
                            }))}
                          />
                        </Field>
                        <div className={styles.saveActions}>
                          <Button
                            variant="secondaryGray"
                            size="sm"
                            disabled={state.busy}
                            onClick={() => setRollbackOpen(false)}
                          >
                            {t("cancel")}
                          </Button>
                          <Button
                            variant="primary"
                            size="sm"
                            disabled={state.busy || !targetRevision}
                            onClick={() =>
                              void state.rollback(Number(targetRevision)).then((success) => {
                                if (success) setRollbackOpen(false);
                              })
                            }
                          >
                            {t("templateVersionRollbackConfirm", { version: targetRevision })}
                          </Button>
                        </div>
                      </>
                    ) : null}
                  </div>
                ) : null}
              </section>
            )}
          </div>
        </div>
      ) : null}
      {state.busy && !state.versions.length ? (
        <p className={styles.loading} role="status">
          <LoaderCircle size={15} className={styles.spinner} aria-hidden="true" />
          {t("templateVersionBusy")}
        </p>
      ) : null}
      {template.uid ? (
        <div className={styles.identity}>
          <span>{t("templateVersionUID")}</span>
          <code>{template.uid}</code>
        </div>
      ) : null}
    </div>
  );
}
