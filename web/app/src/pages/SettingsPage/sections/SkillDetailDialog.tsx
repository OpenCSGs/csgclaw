import { skillOriginHint, skillOriginLabel as originLabel } from "./resourceOrigins";
import { useEffect, useState } from "react";
import { Plus, RefreshCw, Trash2 } from "lucide-react";
import {
  Button,
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
import { isReadonlySkill } from "@/models/skillhub";
import { SkillDetailFiles } from "./SkillDetailFiles";
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
                <div>
                  <div className={styles.titleRow}>
                    <DialogTitle>{skill.name}</DialogTitle>
                    <span className={pageStyles.tag} title={skillOriginHint(skill, t)}>
                      <span className={pageStyles.tagDot} aria-hidden="true" />
                      {originLabel(skill, t)}
                    </span>
                  </div>
                  <DialogDescription>{skill.description || skill.name}</DialogDescription>
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
                <SkillDetailFiles
                  key={JSON.stringify([variant, skill.name, skill.remotePath, skill.remoteRef])}
                  target={{ skill, variant }}
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
                      {t("resourcesDeleteSkill")}
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
              <DialogTitle>{t("resourcesDeleteSkill")}</DialogTitle>
              <DialogDescription>
                {t("resourcesDeleteSkillConfirmMessage", { name: skill?.name || "" })}
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
              {t("resourcesDeleteSkillConfirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </>
  );
}
