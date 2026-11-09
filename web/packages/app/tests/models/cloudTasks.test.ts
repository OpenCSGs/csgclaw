import { describe, expect, it } from "vitest";
import { hasPendingCloudTask } from "@/models/cloudTasks";
import type { IMConversation } from "@/models/conversations";
const room: IMConversation = {
  id: "task",
  created_by: "user-admin",
  members: ["user-admin", "user-cloud"],
  cloud_agent: { agent_id: "agt", connection_id: "conn", user_id: "user-cloud" },
  messages: [
    { id: "event", kind: "event", sender_id: "user-admin", content: "created" },
    { id: "job", sender_id: "user-admin", content: "work" },
  ],
};
describe("cloud task working state", () => {
  it("retains pending state across reload without a runtime work lease", () =>
    expect(hasPendingCloudTask(room)).toBe(true));
  it("releases only messages with matching cloud replies", () => {
    const reply = { id: "reply", sender_id: "user-cloud", content: "done", client_message_id: "relay-reply:job" };
    expect(hasPendingCloudTask({ ...room, messages: [...room.messages, reply] })).toBe(false);
    expect(
      hasPendingCloudTask({
        ...room,
        messages: [...room.messages, reply, { id: "next", sender_id: "user-admin", content: "more" }],
      }),
    ).toBe(true);
  });
  it("ignores local rooms and cloud room creation events", () => {
    expect(hasPendingCloudTask({ ...room, cloud_agent: undefined })).toBe(false);
    expect(hasPendingCloudTask({ ...room, messages: room.messages.slice(0, 1) })).toBe(false);
  });
});
