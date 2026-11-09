import { useState } from "react";
import { Button } from "@/components/ui";
import { agentToDraft, isAgentProfileDraftComplete, type AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { MANAGER_SETUP_DISMISSED_STORAGE_KEY } from "@/shared/storage/keys";
import { writeWorkspacePreference } from "@/shared/storage/workspacePreferences";
import styles from "./ManagerSetupNotice.module.css";

function setupDismissed() {
  try {
    return window.localStorage.getItem(MANAGER_SETUP_DISMISSED_STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

export function ManagerSetupNotice({
  manager,
  onConfigure,
  t,
}: {
  manager: AgentLike | undefined;
  onConfigure: (manager: AgentLike) => void;
  t: TranslateFn;
}) {
  const [dismissed, setDismissed] = useState(setupDismissed);
  const incomplete = manager && !isAgentProfileDraftComplete(agentToDraft(manager));
  if (!incomplete || dismissed) return null;

  return (
    <section className={styles.notice} aria-label={t("managerSetupTitle")}>
      <strong className={styles.title}>{t("managerSetupTitle")}</strong>
      <p className={styles.description}>{t("managerSetupDescription")}</p>
      <div className={styles.actions}>
        <Button size="sm" onClick={() => onConfigure(manager)}>
          {t("managerSetupAction")}
        </Button>
        <Button
          size="sm"
          variant="tertiaryGray"
          onClick={() => {
            setDismissed(true);
            writeWorkspacePreference(MANAGER_SETUP_DISMISSED_STORAGE_KEY, "true");
          }}
        >
          {t("managerSetupLater")}
        </Button>
      </div>
    </section>
  );
}
