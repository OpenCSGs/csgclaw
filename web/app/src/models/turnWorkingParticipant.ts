import { localIdentitiesMatch, threadRootID, type IMMessage } from "./conversations";
import { parseTurnProgress, progressActive } from "./turnProgress";
import type { ConversationWorkingParticipant } from "@/components/business/ConversationPane/types";

// Match the admitted request, sender and thread, never just the latest message
// from an agent. The stop callback still uses the authoritative live lease.
export function workingParticipantForProgress(
  message: IMMessage,
  roomID: string,
  participants: readonly ConversationWorkingParticipant[],
): ConversationWorkingParticipant | undefined {
  const progress = parseTurnProgress(message);
  if (!progress || !progressActive(progress)) return undefined;
  let requestID = "";
  for (const key of ["csgclaw", "codex", "openclaw"]) {
    const metadata = message.metadata?.[key];
    if (
      metadata &&
      typeof metadata === "object" &&
      "request_id" in metadata &&
      typeof metadata.request_id === "string" &&
      metadata.request_id
    ) {
      requestID = metadata.request_id;
      break;
    }
  }
  if (!requestID) return undefined;
  const matches = participants.filter(
    (participant) =>
      participant.roomID === roomID &&
      participant.requestID === requestID &&
      localIdentitiesMatch(participant.id, message.sender_id) &&
      (participant.threadRootID || "") === threadRootID(message),
  );
  return matches.length === 1 ? matches[0] : undefined;
}
