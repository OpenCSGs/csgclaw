import { Outlet } from "react-router-dom";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { classNames } from "@/shared/lib/classNames";

export function WorkspaceMainPanel() {
  const controller = useWorkspaceControllerContext();

  return (
    <main className={classNames("chat-panel", controller.mainPanelHasThread && "has-thread-panel")}>
      <Outlet />
    </main>
  );
}
