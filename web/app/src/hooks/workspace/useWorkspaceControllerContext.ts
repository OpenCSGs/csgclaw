import { useContext } from "react";
import { WorkspaceControllerContext } from "./workspaceControllerContextValue";
import type { WorkspaceController } from "./workspaceControllerContextValue";

export function useOptionalWorkspaceControllerContext(): WorkspaceController | null {
  return useContext(WorkspaceControllerContext);
}

export function useWorkspaceControllerContext(): WorkspaceController {
  const controller = useOptionalWorkspaceControllerContext();
  if (!controller) {
    throw new Error("Workspace controller is not available.");
  }
  return controller;
}
