import { TaskConversationList } from "@/components/business/TaskConversationList";
import { useArchiveConfirmation } from "@/hooks/workspace/useArchiveConfirmation";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";
export function ArchivedSection() {
  const controller = useWorkspaceControllerContext();
  const [skip, setSkip] = useArchiveConfirmation();
  const sidebar = controller.sidebarProps;
  if (!controller.ready || !sidebar || !controller.taskEntry) return null;
  return (
    <SettingsRow title={sidebar.t("iaSettingsCardArchived")} description={sidebar.t("iaSettingsCardArchivedHint")}>
      <label className={styles.archivePreference}>
        <input type="checkbox" checked={skip} onChange={(event) => setSkip(event.target.checked)} />
        {sidebar.t("taskEntryDontAsk")}
      </label>
      <TaskConversationList
        archived
        entry={controller.taskEntry}
        currentUserID={sidebar.currentUserID}
        usersById={sidebar.usersById}
        t={sidebar.t}
        onSelect={sidebar.onSelectConversation}
      />
    </SettingsRow>
  );
}
