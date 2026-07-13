import { useContext } from "react";
import { WorkspaceControllerContext } from "./workspaceControllerContextValue";
import type { WorkspaceController } from "./workspaceControllerContextValue";

export function useWorkspaceControllerContext(): WorkspaceController {
  const controller = useContext(WorkspaceControllerContext);
  if (!controller) {
    throw new Error("Workspace controller is not available.");
  }
  return controller;
}
