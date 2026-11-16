import { post } from "./client";
import type { CompactionOperation } from "@/models/contextCompaction";

export function compactRoomContext(
  roomID: string,
  agentIDs: string[],
  requestID: string,
): Promise<CompactionOperation> {
  return post(`api/v1/rooms/${encodeURIComponent(roomID)}/context:compact`, {
    agent_ids: agentIDs,
    request_id: requestID,
  });
}
