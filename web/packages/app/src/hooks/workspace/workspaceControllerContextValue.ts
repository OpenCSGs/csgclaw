import { createContext, type RefObject } from "react";
import type { WorkspaceController } from "./workspaceControllerContract";
export type { WorkspaceController } from "./workspaceControllerContract";

export const FeedbackTriggerContext = createContext<RefObject<HTMLElement | null> | null>(null);

export const WorkspaceControllerContext = createContext<WorkspaceController | null>(null);
