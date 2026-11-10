import chatPanelStyles from "@/components/business/ConversationPane/ChatPanel.module.css";
import styles from "./WorkspaceMainPanel.module.css";
import { Outlet } from "react-router-dom";

export function WorkspaceMainPanel() {
  return (
    <main className={`chat-panel ${chatPanelStyles["chat-panel"]} ${styles.panel}`}>
      <Outlet />
    </main>
  );
}
