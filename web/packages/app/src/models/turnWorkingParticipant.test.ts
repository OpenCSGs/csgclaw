import { describe, expect, it } from "vitest";
import { workingParticipantForProgress } from "./turnWorkingParticipant";
import type { IMMessage } from "./conversations";
import type { ConversationWorkingParticipant } from "@/components/business/ConversationPane/types";
const message: IMMessage = {
  id: "turn-final",
  sender_id: "user-manager",
  content: "Working",
  metadata: {
    codex: { request_id: "request" },
    csgclaw: {
      turn_progress: { id: "turn", status: "running", revision: 1, started_at: "2026-09-28T00:00:00Z", items: [] },
    },
  },
};
const participant: ConversationWorkingParticipant = {
  id: "user-manager",
  name: "manager",
  participantID: "pt-manager",
  roomID: "room",
  requestID: "request",
  leaseID: "lease",
  canStop: true,
  showContextUsage: true,
};
describe("turn control binding", () => {
  it("binds only the matching request, sender, room and thread", () => {
    const others = [
      { ...participant, requestID: "other" },
      { ...participant, id: "user-worker" },
      { ...participant, roomID: "other" },
      { ...participant, threadRootID: "thread" },
    ];
    expect(workingParticipantForProgress(message, "room", others)).toBeUndefined();
    expect(workingParticipantForProgress(message, "room", [...others, participant])).toBe(participant);
  });
  it("never attaches a new turn's stop control to a completed message", () => {
    const ended = {
      ...message,
      metadata: {
        ...message.metadata,
        csgclaw: {
          turn_progress: {
            id: "turn",
            status: "succeeded",
            revision: 2,
            started_at: "2026-09-28T00:00:00Z",
            items: [],
          },
        },
      },
    };
    expect(workingParticipantForProgress(ended, "room", [participant])).toBeUndefined();
  });
  it("matches thread replies and declines ambiguous or unidentified requests", () => {
    const reply = { ...message, relates_to: { rel_type: "m.thread", event_id: "thread" } };
    const threadParticipant = { ...participant, threadRootID: "thread" };
    expect(workingParticipantForProgress(reply, "room", [participant, threadParticipant])).toBe(threadParticipant);
    expect(
      workingParticipantForProgress(message, "room", [participant, { ...participant, leaseID: "second" }]),
    ).toBeUndefined();
    expect(
      workingParticipantForProgress({ ...message, metadata: { csgclaw: message.metadata?.csgclaw } }, "room", [
        participant,
      ]),
    ).toBeUndefined();
  });
});
