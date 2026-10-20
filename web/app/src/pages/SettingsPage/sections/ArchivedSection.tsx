import { TaskConversationList } from "@/components/business/TaskConversationList";
import { useArchiveConfirmation } from "@/hooks/workspace/useArchiveConfirmation";
import { useTaskConversationPages } from "@/hooks/workspace/useTaskConversationPages";
import { Switch } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";
export function ArchivedSection() {
  const controller = useWorkspaceControllerContext();
  const [skip, setSkip] = useArchiveConfirmation();
  const sidebar = controller.sidebarProps;
  const catalog = useTaskConversationPages(true, "", Boolean(controller.ready && controller.taskEntry && sidebar));
  const count = catalog.data?.pages[0]?.total;
  if (!controller.ready || !sidebar || !controller.taskEntry) return null;
  return (
    <SettingsRow
      title={sidebar.t("iaSettingsCardArchived")}
      description={sidebar.t("archivedDescription")}
      actions={
        typeof count === "number" ? (
          <span className={styles.countBadge}>{sidebar.t("archivedCount", { count })}</span>
        ) : undefined
      }
    >
      <div className={styles.archivePreference}>
        <div>
          <strong>{sidebar.t("archivedConfirmSwitch")}</strong>
          <p>{sidebar.t("archivedConfirmSwitchHint")}</p>
        </div>
        <Switch
          checked={!skip}
          onCheckedChange={(checked) => setSkip(!checked)}
          aria-label={sidebar.t("archivedConfirmSwitch")}
        />
      </div>
      <TaskConversationList
        archived
        entry={controller.taskEntry}
        currentUserID={sidebar.currentUserID}
        usersById={sidebar.usersById}
        t={sidebar.t}
        onSelect={sidebar.onSelectConversation}
      />
      <p className={styles.listNotice}>{sidebar.t("archivedWhereHint")}</p>
    </SettingsRow>
  );
}
