import { skillOriginHint, skillOriginLabel as originLabel } from "./resourceOrigins";
import { useEffect, useState } from "react";
import { Check, Copy, Folder, Plus, RefreshCw, Trash2 } from "lucide-react";
import {
  Button,
  IconButton,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Switch,
} from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { writeTextToClipboard } from "@/shared/lib/clipboard";
import { SkillSourceDetails } from "./SkillSourceDetails";
import { skillDirectory, skillVersionLabel } from "@/models/skillLibrary";
import { isReadonlySkill } from "@/models/skillhub";
import { SkillDetailFiles } from "@/components/business/SkillDetailFiles";
import styles from "./SkillDetailDialog.module.css";
import pageStyles from "../SettingsPage.module.css";
import type { SkillSummary } from "@/models/skillhub";
import type { TranslateFn } from "@/models/conversations";

export type SkillDetailTarget = {
  skill: SkillSummary;
  variant: "installed" | "market";
};

export function SkillDetailDialog({
  target,
  t,
  onOpenChange,
  market,
  toggle,
  allowDelete = true,
  portalContainer,
  removeAction,
}: {
  target: SkillDetailTarget | null;
  t: TranslateFn;
  onOpenChange: (open: boolean) => void;
  market?: {
    installed: boolean;
    busy: boolean;
    onAction: () => void;
  };
  /** Enables or disables this resource for the current agent. */
  toggle?: { enabled: boolean; busy: boolean; onToggle: () => void };

  allowDelete?: boolean;
  portalContainer?: HTMLElement | null;

  removeAction?: { busy: boolean; onRemove: () => void };
}) {
  const controller = useWorkspaceControllerContext();
  const hub = controller.hubViewProps?.hub;
  const skill = target?.skill ?? null;
  const variant = target?.variant ?? "installed";
  const [confirmOpen, setConfirmOpen] = useState(false);
  // Keep deletion errors local; hub errors depend on the selected resource type.
  const [deleteError, setDeleteError] = useState("");

  useEffect(() => {
    if (!skill) {
      setConfirmOpen(false);
      setDeleteError("");
      return;
    }
  }, [skill]);

  const canDelete = allowDelete && variant === "installed" && Boolean(skill) && !isReadonlySkill(skill);

  async function handleDelete() {
    if (!skill) {
      return;
    }
    // Suppress resource navigation so deletion keeps the settings dialog open.
    setDeleteError("");
    const deleted = await hub?.deleteSkill?.(skill, { navigate: false });
    if (deleted) {
      setConfirmOpen(false);
      onOpenChange(false);
    } else {
      setDeleteError(t("resourcesSkillDeleteFailed"));
    }
  }

  return (
    <>
      <DialogRoot open={Boolean(skill)} onOpenChange={onOpenChange}>
        <DialogContent className={styles.dialog} portalContainer={portalContainer}>
          {skill ? (
            <>
              <DialogHeader>
                <div className={styles.headingCopy}>
                  <div className={styles.titleRow}>
                    <DialogTitle>{skill.name}</DialogTitle>
                    <span className={pageStyles.tag} title={skillOriginHint(skill, t)}>
                      {skill.discovery
                        ? t("iaSkillsActiveVersion", {
                            version: skillVersionLabel(skill.discovery.version) || t("iaSkillsUnversioned"),
                          })
                        : originLabel(skill, t)}
                    </span>
                  </div>
                  <SkillDescription key={skill.name} text={skill.description || skill.name} t={t} />
                  {variant === "installed" ? (
                    <SkillLocation key={skillDirectory(skill)} directory={skillDirectory(skill)} t={t} />
                  ) : null}
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  {toggle ? (
                    <Switch
                      aria-label={skill.name}
                      checked={toggle.enabled}
                      disabled={toggle.busy}
                      onCheckedChange={() => toggle.onToggle()}
                    />
                  ) : null}
                  <DialogCloseButton label={t("close")} size="md" variant="tertiaryGray" />
                </div>
              </DialogHeader>
              <DialogBody className={styles.body}>
                {variant === "installed" && skill.discovery ? (
                  <SkillSourceDetails skill={skill} locale={controller.sidebarProps?.locale || "zh"} t={t} />
                ) : null}
                <SkillDetailFiles
                  key={JSON.stringify([variant, skill.name, skill.discovery?.key, skill.remotePath, skill.remoteRef])}
                  skill={skill}
                  market={variant === "market"}
                  t={t}
                />
              </DialogBody>
              {canDelete || removeAction || (variant === "market" && market) ? (
                <DialogFooter className={styles.footer}>
                  {removeAction ? (
                    <Button
                      className={styles.footerDelete}
                      variant="danger"
                      size="sm"
                      loading={removeAction.busy}
                      onClick={() => removeAction.onRemove()}
                    >
                      <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
                      {t("agentRemoveSkillFromAgent")}
                    </Button>
                  ) : null}

                  {canDelete ? (
                    <Button
                      className={styles.footerDelete}
                      variant="danger"
                      size="sm"
                      disabled={Boolean(hub?.skillDeleteBusy)}
                      onClick={() => setConfirmOpen(true)}
                    >
                      <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
                      {t(skill?.discovery ? "iaSkillsRemoveLinked" : "resourcesDeleteSkill")}
                    </Button>
                  ) : null}
                  {variant === "market" && market ? (
                    <Button variant="primary" size="sm" loading={market.busy} onClick={market.onAction}>
                      {market.installed ? (
                        <RefreshCw size={15} strokeWidth={2} aria-hidden="true" />
                      ) : (
                        <Plus size={15} strokeWidth={2.2} aria-hidden="true" />
                      )}
                      {market.installed ? t("iaSkillsUpdate") : t("resourcesSkillRemoteInstallAction")}
                    </Button>
                  ) : null}
                </DialogFooter>
              ) : null}
            </>
          ) : null}
        </DialogContent>
      </DialogRoot>

      {/* Separate dialog roots prevent competing focus scopes during confirmation. */}
      <DialogRoot open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent className={styles.confirmDialog}>
          <DialogHeader>
            <div>
              <DialogTitle>{t(skill?.discovery ? "iaSkillsRemoveLinked" : "resourcesDeleteSkill")}</DialogTitle>
              <DialogDescription>
                {t(skill?.discovery ? "iaSkillsRemoveLinkedHint" : "resourcesDeleteSkillConfirmMessage", {
                  name: skill?.name || "",
                })}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {deleteError ? (
            <p className={styles.confirmError} role="alert">
              {deleteError}
            </p>
          ) : null}
          <DialogFooter>
            <Button variant="secondaryGray" size="sm" onClick={() => setConfirmOpen(false)}>
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              size="sm"
              loading={Boolean(hub?.skillDeleteBusy)}
              onClick={() => void handleDelete()}
            >
              {t(skill?.discovery ? "iaSkillsRemoveLinked" : "resourcesDeleteSkillConfirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </>
  );
}

function SkillLocation({ directory, t }: { directory: string; t: TranslateFn }) {
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);
  if (!directory) return <p className={styles.embedded}>{t("iaSkillsEmbeddedDirectory")}</p>;
  return (
    <div className={styles.location}>
      <span className={styles.locationLabel}>
        <Folder size={14} aria-hidden="true" />
        {t("iaSkillsDirectory")}
      </span>
      <code className={styles.locationPath}>{directory}</code>
      <IconButton
        variant="tertiaryGray"
        size="sm"
        label={t(copied ? "copiedToClipboard" : "iaSkillsCopyDirectory")}
        icon={copied ? <Check size={15} /> : <Copy size={15} />}
        onClick={async () => {
          const success = await writeTextToClipboard(directory);
          setCopied(success);
          setFailed(!success);
        }}
      />
      {failed ? (
        <span className={styles.locationError} role="alert">
          {t("iaSkillsCopyDirectoryFailed")}
        </span>
      ) : null}
    </div>
  );
}

function SkillDescription({ text, t }: { text: string; t: TranslateFn }) {
  const [expanded, setExpanded] = useState(false);
  return (
    <>
      <DialogDescription className={expanded ? styles.fullDescription : styles.description}>{text}</DialogDescription>
      {text.length > 240 ? (
        <Button variant="linkGray" size="sm" onClick={() => setExpanded(!expanded)} aria-expanded={expanded}>
          {t(expanded ? "iaSkillsDescriptionLess" : "iaSkillsDescriptionMore")}
        </Button>
      ) : null}
    </>
  );
}
