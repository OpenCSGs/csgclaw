import { createContext } from "react";
import type { WorkspaceController } from "./workspaceControllerContract";
export type { WorkspaceController } from "./workspaceControllerContract";

export const WorkspaceControllerContext = createContext<WorkspaceController | null>(null);
