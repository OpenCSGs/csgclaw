// @ts-nocheck
import { Outlet } from "react-router-dom";

export function WorkspaceMainPanel() {
  return (
    <main className="chat-panel">
      <Outlet />
    </main>
  );
}
