import { createContext, useContext } from "react";
import type { ReactNode } from "react";
import type { useWorkspaceController } from "./useWorkspaceController";

export type WorkspaceController = ReturnType<typeof useWorkspaceController>;

const WorkspaceControllerContext = createContext<WorkspaceController | null>(null);

export function WorkspaceControllerProvider({
  controller,
  children,
}: {
  children: ReactNode;
  controller: WorkspaceController;
}) {
  return <WorkspaceControllerContext.Provider value={controller}>{children}</WorkspaceControllerContext.Provider>;
}

export function useWorkspaceControllerContext(): WorkspaceController {
  const controller = useContext(WorkspaceControllerContext);
  if (!controller) {
    throw new Error("Workspace controller is not available.");
  }
  return controller;
}
