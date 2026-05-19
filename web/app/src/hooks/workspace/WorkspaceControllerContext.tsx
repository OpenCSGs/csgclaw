import { createContext, useContext } from "react";

const WorkspaceControllerContext = createContext(null);

export function WorkspaceControllerProvider({ controller, children }) {
  return <WorkspaceControllerContext.Provider value={controller}>{children}</WorkspaceControllerContext.Provider>;
}

export function useWorkspaceControllerContext() {
  const controller = useContext(WorkspaceControllerContext);
  if (!controller) {
    throw new Error("Workspace controller is not available.");
  }
  return controller;
}
