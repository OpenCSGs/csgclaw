import { createContext } from "react";
import type { useWorkspaceController } from "./useWorkspaceController";

export type WorkspaceController = ReturnType<typeof useWorkspaceController>;

export const WorkspaceControllerContext = createContext<WorkspaceController | null>(null);
