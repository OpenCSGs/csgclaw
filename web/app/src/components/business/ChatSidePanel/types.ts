import type { ComposerFileReference } from "@/models/fileReferences";
import type { IMMessage } from "@/models/conversations";

export type ChatFileAgent = { id: string; name: string };
export type ChatFileScope = {
  roomID: string;
  userID: string;
  agents: ChatFileAgent[];
  defaultAgentID: string;
  retainedMessages?: IMMessage[];
  messages?: IMMessage[];
};
export type ReferenceFile = (reference: ComposerFileReference) => void;
