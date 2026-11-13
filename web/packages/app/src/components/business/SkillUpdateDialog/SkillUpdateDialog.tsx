import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, RefreshCw } from "lucide-react";
import { applySkillUpdates, previewSkillUpdate } from "@/api/skills";
import { skillVersionLabel } from "@/models/skillLibrary";
import { skillUpdateErrorKey } from "@/models/skillUpdates";
import type { SkillUpdateInfo, SkillUpdateResult, SkillUpdateStrategy, SkillUpdateTarget } from "@/models/skillUpdates";
import { skillUpdatesQueryKey } from "@/hooks/workspace/useSkillUpdates";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import {
  Button,
  InlineError,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogCloseButton,
  DialogBody,
  DialogFooter,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import styles from "./SkillUpdateDialog.module.css";

export function SkillUpdateDialog({
  names,
  t,
  onClose,
  locale = "zh",
}: {
  names: string[];
  t: TranslateFn;
  onClose: (updated: boolean) => void;
  locale?: string;
}) {
  const client = useQueryClient();
  const [strategy, setStrategy] = useState<SkillUpdateStrategy>("update");
  const [busy, setBusy] = useState(false);
  const [results, setResults] = useState<SkillUpdateResult[]>([]);
  const [failure, setFailure] = useState("");
  const preview = useQuery({
    queryKey: ["skill-update-preview", ...names],
    queryFn: async () => {
      const items: SkillUpdateInfo[] = [];
      for (let offset = 0; offset < names.length; offset += 4) {
        items.push(...(await Promise.all(names.slice(offset, offset + 4).map(previewSkillUpdate))));
      }
      return items;
    },
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const items = preview.data ?? [];
  const ready =
    items.length === names.length && items.every((item) => item.commit && item.state && item.targets.length > 0);
  const completed = results.length > 0 && results.every((result) => result.status !== "failed");
  const reinstall =
    ready && items.every((item) => item.targets.every((target) => sameVersion(target.version, item.latestVersion)));
  const anyUpdated = results.some((result) => result.status === "updated");
  const error = failure || (preview.error ? t(skillUpdateErrorKey((preview.error as { code?: string }).code)) : "");

  async function refresh() {
    setStrategy("update");
    setResults([]);
    setFailure("");
    await preview.refetch();
  }

  async function apply() {
    if (!ready || busy || preview.isFetching || preview.isError) return;
    if (reinstall && strategy === "keep-local") {
      onClose(false);
      return;
    }
    setBusy(true);
    setFailure("");
    try {
      const response = await applySkillUpdates(items, strategy);
      setResults(response.results);
      if (response.results.length === 0) setFailure(t("skillUpdatesFailed"));
      await Promise.all([
        client.invalidateQueries({ queryKey: skillUpdatesQueryKey }),
        client.invalidateQueries({ queryKey: workspaceQueryKeys.skills() }),
        client.invalidateQueries({ queryKey: workspaceQueryKeys.agents() }),
        client.invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills("").slice(0, 2) }),
        client.invalidateQueries({ queryKey: workspaceQueryKeys.skillTreeScope() }),
        client.invalidateQueries({ queryKey: workspaceQueryKeys.skillFileScope() }),
        client.invalidateQueries({ queryKey: workspaceQueryKeys.agentWorkspaceScope("").slice(0, 2) }),
        client.invalidateQueries({ queryKey: workspaceQueryKeys.agentWorkspaceFileScope("").slice(0, 2) }),
      ]);
    } catch (err) {
      setFailure(t(skillUpdateErrorKey((err as { code?: string }).code)));
    } finally {
      setBusy(false);
    }
  }

  const close = () => {
    if (!busy) onClose(anyUpdated);
  };
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent
        className={styles.dialog}
        scrollBody
        onEscapeKeyDown={(event) => {
          if (busy) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (busy) event.preventDefault();
        }}
      >
        <DialogHeader>
          <div>
            <DialogTitle>
              {t(reinstall ? "skillUpdatesReinstallTitle" : "skillUpdatesConfirmTitle", { count: names.length })}
            </DialogTitle>
            <DialogDescription>{t("skillUpdatesNoOverwrite")}</DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} disabled={busy} size="sm" variant="tertiaryGray" />
        </DialogHeader>
        <DialogBody className={styles.body}>
          {preview.isFetching ? <p role="status">{t("skillUpdatesPreparing")}</p> : null}
          {items.map((item) => (
            <section key={item.name} className={styles.skill} aria-label={item.name}>
              <div className={styles.versionRow}>
                <strong>{item.name}</strong>
                <VersionLine
                  current={item.targets[0]?.version ?? item.localVersion}
                  latest={item.latestVersion}
                  t={t}
                />
              </div>
              <p className={styles.hint}>{t("skillUpdatesImpact", { count: item.targets.length })}</p>
              {!reinstall && item.publishedAt && !item.publishedAt.startsWith("0001-") ? (
                <p className={styles.hint}>
                  {t("skillUpdatesPublished", {
                    date: new Date(item.publishedAt).toLocaleDateString(locale === "zh" ? "zh-CN" : "en-US"),
                  })}
                </p>
              ) : null}
              {!reinstall ? (
                <details>
                  <summary>{t("skillUpdatesReleaseNotes")}</summary>
                  <p className={styles.changelog}>{item.changelog || t("skillUpdatesNoNotes")}</p>
                </details>
              ) : null}
              {item.targets.map((target) => (
                <TargetPreview key={target.id} target={target} latest={item.latestVersion} t={t} />
              ))}
              {!item.commit ? <InlineError>{t("skillUpdatesNoRelease")}</InlineError> : null}
              {item.commit && item.targets.length === 0 ? (
                <p className={styles.hint}>{t("skillUpdatesNewerCopies")}</p>
              ) : null}
            </section>
          ))}
          {ready && !completed && results.length === 0 ? (
            <fieldset className={styles.choices} disabled={busy}>
              <legend>{t(reinstall ? "skillUpdatesReinstallQuestion" : "skillUpdatesQuestion")}</legend>
              {(["update", "keep-local"] as const).map((choice) => (
                <label key={choice} className={styles.choice} data-selected={strategy === choice}>
                  <input
                    type="radio"
                    name="skill-update-strategy"
                    checked={strategy === choice}
                    onChange={() => setStrategy(choice)}
                  />
                  <span>
                    <strong>
                      {t(
                        reinstall
                          ? choice === "update"
                            ? "skillUpdatesReinstall"
                            : "skillUpdatesReinstallKeep"
                          : choice === "update"
                            ? "skillUpdatesChoiceUpdate"
                            : "skillUpdatesChoiceKeep",
                      )}
                    </strong>
                    <small>
                      {t(
                        reinstall
                          ? choice === "update"
                            ? "skillUpdatesReinstallHint"
                            : "skillUpdatesReinstallKeepHint"
                          : choice === "update"
                            ? "skillUpdatesOverwriteHint"
                            : "skillUpdatesKeepHint",
                      )}
                    </small>
                  </span>
                </label>
              ))}
            </fieldset>
          ) : null}
          {error ? <InlineError role="alert">{error}</InlineError> : null}
          {results.length > 0 ? (
            <div role="status" className={styles.results}>
              {results.map((result, index) => (
                <p key={index}>
                  <strong>
                    {result.name}
                    {result.targetId && result.targetId !== "library" ? ` · ${result.targetName}` : ""}
                  </strong>
                  :{" "}
                  <span>
                    {t(
                      result.status === "failed"
                        ? skillUpdateErrorKey(result.error)
                        : result.status === "kept"
                          ? "skillUpdatesKept"
                          : "skillUpdatesDone",
                    )}
                  </span>
                </p>
              ))}
            </div>
          ) : null}
        </DialogBody>
        <DialogFooter className={styles.footer}>
          <Button variant="secondaryGray" size="sm" disabled={busy} onClick={close}>
            {t(completed ? "close" : "cancel")}
          </Button>
          {(error || results.some((result) => result.status === "failed")) && !busy ? (
            <Button size="sm" variant="secondaryGray" onClick={() => void refresh()}>
              <RefreshCw size={14} aria-hidden="true" />
              {t("skillUpdatesCheckAgain")}
            </Button>
          ) : null}
          {!completed ? (
            <Button
              variant="primary"
              size="sm"
              loading={busy}
              disabled={!ready || preview.isFetching || preview.isError || results.length > 0}
              onClick={() => void apply()}
            >
              {t(
                reinstall && strategy !== "keep-local"
                  ? "skillUpdatesConfirmReinstall"
                  : strategy === "keep-local"
                    ? "skillUpdatesConfirmKeep"
                    : "skillUpdatesConfirmUpdate",
              )}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

export function VersionLine({ current, latest, t }: { current?: string; latest?: string; t: TranslateFn }) {
  return (
    <span className={styles.versions}>
      <span>{skillVersionLabel(current) || t("iaSkillsUnversioned")}</span>
      {!sameVersion(current, latest) ? (
        <>
          <ArrowRight size={14} aria-hidden="true" />
          <strong>{skillVersionLabel(latest) || t("iaSkillsUnversioned")}</strong>
        </>
      ) : null}
    </span>
  );
}

function sameVersion(current?: string, latest?: string) {
  return Boolean(current && latest && current.replace(/^v/, "") === latest.replace(/^v/, ""));
}

function TargetPreview({ target, latest, t }: { target: SkillUpdateTarget; latest: string; t: TranslateFn }) {
  return (
    <details className={styles.target}>
      <summary>
        <span>{target.id === "library" ? t("skillUpdatesLibrary") : target.name}</span>
        <VersionLine current={target.version} latest={latest} t={t} />
        <span>{t("skillUpdatesChanges", { count: target.changes?.length ?? 0 })}</span>
      </summary>
      {(target.changes ?? []).map((change) => (
        <details key={change.path} className={styles.file}>
          <summary>
            <span>{t(`skillUpdatesFile_${change.kind}`)}</span>
            <code>{change.path}</code>
          </summary>
          {change.previewUnavailable ? (
            <p>{t("skillUpdatesPreviewUnavailable")}</p>
          ) : (
            <div className={styles.diff}>
              <div>
                <span>{t("skillUpdatesCurrentContent")}</span>
                <pre>{change.before || t("skillUpdatesEmptyContent")}</pre>
              </div>
              <div>
                <span>{t("skillUpdatesNewContent")}</span>
                <pre>{change.after || t("skillUpdatesEmptyContent")}</pre>
              </div>
            </div>
          )}
        </details>
      ))}
    </details>
  );
}
