import { useRef, type ReactNode } from "react";
import { FeedbackTriggerContext, WorkspaceControllerContext } from "./workspaceControllerContextValue";
import type { WorkspaceController } from "./workspaceControllerContextValue";

export type { WorkspaceController } from "./workspaceControllerContextValue";

export function WorkspaceControllerProvider({
  controller,
  children,
}: {
  children: ReactNode;
  controller: WorkspaceController;
}) {
  const feedbackTrigger = useRef<HTMLElement | null>(null);
  return (
    <WorkspaceControllerContext.Provider value={controller}>
      <FeedbackTriggerContext.Provider value={feedbackTrigger}>{children}</FeedbackTriggerContext.Provider>
    </WorkspaceControllerContext.Provider>
  );
}
