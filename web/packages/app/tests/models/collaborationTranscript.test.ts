import { describe, expect, it } from "vitest";
import { collaborationTranscript, usesCollaborationTranscript } from "@/models/collaborationTranscript";
import { expandProgressMessage, type TurnProgress } from "@/models/turnProgress";
import { groupVideoGenerationMessages, type IMMessage } from "@/models/conversations";

function turn(id: string, content: string, status = "succeeded", ended = "2026-11-11T12:11:00Z"): IMMessage {
  return {
    id,
    sender_id: "dev",
    content,
    created_at: "2026-11-11T12:04:00Z",
    metadata: {
      csgclaw: {
        turn_progress: {
          id,
          revision: 1,
          status,
          started_at: "2026-11-11T12:04:00Z",
          ended_at: ended,
          items: [],
        },
      },
    },
  };
}

describe("collaboration transcript", () => {
  it("keeps public answers after earlier acknowledgments without mutating stored messages", () => {
    const answer = turn("answer", "Final answer");
    const acknowledgment = {
      id: "claim",
      content: "I will check",
      sender_id: "dev",
      created_at: "2026-11-11T12:05:00Z",
    };
    const result = collaborationTranscript([answer, acknowledgment]);
    expect(result.messages.map((message) => message.id)).toEqual(["claim", "answer"]);
    expect(result.messages[1].created_at).toBe("2026-11-11T12:11:00Z");
    expect(answer.created_at).toBe("2026-11-11T12:04:00Z");
    expect(result.activities).toEqual([answer]);
  });

  it("retains silent runs and partial output as activity, without empty public bubbles", () => {
    const messages = [
      turn("silent", "\u200b"),
      turn("legacy-silent", " <csgclaw-silent/> \u200b"),
      turn("working", "Unpublished draft", "running"),
      turn("waiting", "", "waiting"),
    ];
    const result = collaborationTranscript(messages);
    expect(result.messages).toEqual([]);
    expect(result.activities).toEqual(messages);
  });

  it("preserves attachment-only answers, failures and invalid timestamp fallback", () => {
    const answer = {
      ...turn("file", "<csgclaw-silent/>", "succeeded", "invalid"),
      attachments: [
        {
          id: "file",
          name: "result.txt",
          kind: "file",
          media_type: "text/plain",
          download_url: "/result.txt",
          created_at: "2026-11-11T12:04:00Z",
          sha256: "sha",
          size_bytes: 10,
        },
      ],
    };
    const failure = turn("failure", "", "failed");
    const result = collaborationTranscript([answer, failure]);
    expect(result.messages).toEqual([{ ...answer, content: "" }]);
    expect(result.activities).toEqual([answer, failure]);
  });

  it("keeps questions and approvals visible while collecting legacy tools", () => {
    const messages = ["tool", "question", "action", "notice"].map((kind) => ({
      id: kind,
      content: kind,
      sender_id: "dev",
      metadata: {
        csgclaw: {
          agent_activity: {
            type: "com.opencsg.csgclaw.agent.activity",
            version: 1,
            channel: "csgclaw",
            room_id: "room",
            sender: "dev",
            event_id: kind,
            origin_server_ts: 1,
            content: {
              msgtype: `com.opencsg.csgclaw.agent.${kind}`,
              body: kind,
              tool: kind === "tool" ? { id: "tool", title: "Read", status: "completed" } : undefined,
            },
          },
        },
      },
    }));
    const result = collaborationTranscript(messages);
    expect(result.messages.map((message) => message.id)).toEqual(["question", "action", "notice"]);
    expect(result.activities.map((message) => message.id)).toEqual(["tool"]);
  });

  it("keeps a late result after a newer human message", () => {
    const result = collaborationTranscript([
      turn("old-run", "Late result"),
      { id: "followup", content: "Add sources", created_at: "2026-11-11T12:06:00Z" },
    ]);
    expect(result.messages.map((message) => message.id)).toEqual(["followup", "old-run"]);
  });

  it("keeps generated media visible while the parent execution is collapsed", () => {
    const video = {
      id: "video",
      sender_id: "dev",
      content: "Generated video",
      metadata: { video_generation: { status: "running" }, codex: { turn_message_id: "parent" } },
    };
    const running = collaborationTranscript([turn("parent", "", "running"), video]);
    expect(running.messages).toEqual([video]);
    expect(groupVideoGenerationMessages(running.messages).childMessageIDs.size).toBe(0);
    const finished = collaborationTranscript([turn("parent", "Here is the video"), video]);
    expect(groupVideoGenerationMessages(finished.messages).childMessageIDs.has("video")).toBe(true);
  });

  it("restores a hidden final reply from an unmentioned peer in the original thread", () => {
    const relation = { rel_type: "m.thread", event_id: "root" };
    const source = { id: "peer", sender_id: "manager", content: "What do you think?", relates_to: relation };
    const hidden = { ...turn("hidden", "\u200b"), relates_to: relation };
    const progress = (hidden.metadata!.csgclaw as { turn_progress: TurnProgress }).turn_progress;
    progress.private_output = true;
    progress.final_text = "Add an independent review.";
    const result = collaborationTranscript([source, hidden]);
    expect(result.messages.map((message) => message.content)).toEqual([source.content, progress.final_text]);
    expect(result.messages[1].relates_to).toEqual(relation);
    expect(hidden.content).toBe("\u200b");
    expect(expandProgressMessage(hidden).at(-1)?.content).toBe(progress.final_text);

    const explicit = {
      id: "sent",
      sender_id: "dev",
      content: progress.final_text,
      created_at: "2026-11-11T12:05:00Z",
      relates_to: relation,
    };
    expect(collaborationTranscript([source, explicit, hidden]).messages).toHaveLength(2);
    expect(collaborationTranscript([source, { ...explicit, relates_to: null }, hidden]).messages).toHaveLength(3);
    expect(
      collaborationTranscript([source, { ...explicit, created_at: "2026-11-11T12:03:00Z" }, hidden]).messages,
    ).toHaveLength(3);
    progress.final_text = "<csgclaw-silent/>";
    expect(collaborationTranscript([source, hidden]).messages).toEqual([source]);
  });

  it("does not enable the projection for direct or cloud conversations", () => {
    const room = { id: "room", members: [], messages: [] };
    expect(usesCollaborationTranscript(room)).toBe(true);
    expect(usesCollaborationTranscript({ ...room, is_direct: true })).toBe(false);
    expect(
      usesCollaborationTranscript({
        ...room,
        cloud_agent: { agent_id: "cloud", connection_id: "connection", user_id: "user" },
      }),
    ).toBe(false);
  });
});
