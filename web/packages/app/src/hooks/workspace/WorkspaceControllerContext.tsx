import type { ReactNode } from "react";
import { WorkspaceControllerContext } from "./workspaceControllerContextValue";
import type { WorkspaceController } from "./workspaceControllerContextValue";

export type { WorkspaceController } from "./workspaceControllerContextValue";

export function WorkspaceControllerProvider({
  controller,
  children,
}: {
  children: ReactNode;
  controller: WorkspaceController;
}) {
  return <WorkspaceControllerContext.Provider value={controller}>{children}</WorkspaceControllerContext.Provider>;
}
