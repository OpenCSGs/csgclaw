import { useState } from "react";
import { TaskConversationList } from "@/components/business/TaskConversationList";
import { useArchiveConfirmation } from "@/hooks/workspace/useArchiveConfirmation";
import { useTaskConversationPages } from "@/hooks/workspace/useTaskConversationPages";
import { SegmentedControl, Switch } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { ArchivedAgents } from "./ArchivedAgents";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";
export function ArchivedSection() {
  const controller = useWorkspaceControllerContext();
  const [skip, setSkip] = useArchiveConfirmation();
  const [tab, setTab] = useState<"sessions" | "agents">("sessions");
  const [agentCount, setAgentCount] = useState(0);
  const sidebar = controller.sidebarProps;
  const catalog = useTaskConversationPages(true, "", Boolean(controller.ready && controller.taskEntry && sidebar));
  const count = tab === "agents" ? agentCount : catalog.data?.pages[0]?.total;
  if (!controller.ready || !sidebar || !controller.taskEntry) return null;
  return (
    <SettingsRow
      title={sidebar.t("iaSettingsCardArchived")}
      description={sidebar.t(tab === "agents" ? "archivedAgentsHint" : "archivedDescription")}
      actions={
        typeof count === "number" ? (
          <span className={styles.countBadge}>{sidebar.t("archivedCount", { count })}</span>
        ) : undefined
      }
    >
      <SegmentedControl
        value={tab}
        onChange={setTab}
        variant="underline"
        label={sidebar.t("iaSettingsCardArchived")}
        options={[
          { value: "sessions", label: sidebar.t("archivedConversationsTab") },
          { value: "agents", label: sidebar.t("archivedAgentsTab") },
        ]}
      />
      {tab === "agents" ? (
        <ArchivedAgents t={sidebar.t} usersById={sidebar.usersById} onCountChange={setAgentCount} />
      ) : (
        <>
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
        </>
      )}
    </SettingsRow>
  );
}
