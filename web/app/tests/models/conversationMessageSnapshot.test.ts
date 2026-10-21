import {
  mergeConversationMessageSnapshot,
  removeUserFromData,
  THREAD_RELATION_TYPE,
  type IMData,
  type IMMessage,
} from "@/models/conversations";

const message = (id: string): IMMessage => ({ id, sender_id: "agent", content: id });
const data = (messages: IMMessage[]): IMData => ({ users: [], rooms: [{ id: "room", members: [], messages }] });
const progress = (revision: number) => ({
  csgclaw: {
    turn_progress: {
      id: "turn",
      revision,
      status: "running",
      started_at: "2026-10-21T00:00:00Z",
      items: [],
    },
  },
});

it("uses snapshot order and retains messages received after the snapshot", () => {
  const current = data([message("newer"), message("live")]);
  const next = mergeConversationMessageSnapshot(current, "room", [message("older"), message("newer")]);
  expect(next?.rooms[0].messages.map((item) => item.id)).toEqual(["older", "newer", "live"]);
  expect(current.rooms[0].messages.map((item) => item.id)).toEqual(["newer", "live"]);
});

it("keeps unchanged data references and ignores unknown rooms", () => {
  const current = data([message("one")]);
  expect(mergeConversationMessageSnapshot(current, "room", [message("one")])).toBe(current);
  expect(mergeConversationMessageSnapshot(current, "missing", [message("one")])).toBe(current);
  expect(mergeConversationMessageSnapshot(null, "room", [])).toBeNull();
});

it("does not duplicate snapshot messages or add thread replies to the main transcript", () => {
  const next = mergeConversationMessageSnapshot(data([]), "room", [
    message("one"),
    message("one"),
    { ...message("thread"), relates_to: { rel_type: THREAD_RELATION_TYPE, event_id: "one" } },
  ]);
  expect(next?.rooms[0].messages.map((item) => item.id)).toEqual(["one"]);
});

it.each([undefined, { root_id: "root", reply_count: 1 }])(
  "keeps a thread update received during the request over snapshot %j",
  (thread) => {
    const before = { ...message("root"), thread };
    const updated = { ...before, thread: { root_id: "root", reply_count: 2 } };
    const next = mergeConversationMessageSnapshot(data([updated]), "room", [before], [before]);
    expect(next?.rooms[0].messages[0].thread).toBe(updated.thread);
  },
);

it.each([2, 3])("merges authoritative thread changes independently of progress revision %s", (revision) => {
  const before = {
    ...message("root"),
    content: "Latest",
    metadata: progress(3),
    thread: { root_id: "root", reply_count: 2 },
  };
  const incoming = {
    ...before,
    content: "Old",
    metadata: progress(revision),
    thread: { root_id: "root", reply_count: 1 },
  };
  const next = mergeConversationMessageSnapshot(data([before]), "room", [incoming], [before]);
  expect(next?.rooms[0].messages[0].content).toBe("Latest");
  expect(next?.rooms[0].messages[0].thread?.reply_count).toBe(1);
});

it.each([0, 1])("accepts reply count %s after deleting a participant, including removal of the last reply", (count) => {
  const root = { ...message("root"), sender_id: "admin", thread: { root_id: "root", reply_count: 2 } };
  const initial: IMData = {
    users: [{ id: "admin" }, { id: "deleted" }, { id: "other" }],
    rooms: [{ id: "room", members: ["admin", "deleted", "other"], messages: [root] }],
  };
  const deleted = removeUserFromData(initial, "deleted");
  const incoming = { ...root, thread: count ? { root_id: "root", reply_count: count } : undefined };
  const next = mergeConversationMessageSnapshot(deleted, "room", [incoming], deleted.rooms[0].messages);
  expect(next?.rooms[0].messages[0].thread?.reply_count ?? 0).toBe(count);
});

it("keeps a same-count latest-reply update received during the request", () => {
  const before = { ...message("root"), thread: { root_id: "root", reply_count: 1, latest_reply: message("reply") } };
  const updated = {
    ...before,
    thread: { ...before.thread, latest_reply: { ...message("reply"), content: "Updated" } },
  };
  const next = mergeConversationMessageSnapshot(data([updated]), "room", [before], [before]);
  expect(next?.rooms[0].messages[0].thread?.latest_reply?.content).toBe("Updated");
});

it("syncs attachment deletion even when the progress revision is unchanged", () => {
  const attachment = {
    id: "file",
    name: "file.txt",
    download_url: "file.txt",
    size_bytes: 1,
    media_type: "text/plain",
    kind: "file",
    sha256: "hash",
    created_at: "2026-10-21T00:00:00Z",
  };
  const before = { ...message("root"), metadata: progress(3), attachments: [attachment] };
  const incoming = { ...before, attachments: [] };
  const next = mergeConversationMessageSnapshot(data([before]), "room", [incoming], [before]);
  expect(next?.rooms[0].messages[0].attachments).toEqual([]);
});

it("preserves attachment deletion received during the request", () => {
  const before = {
    ...message("root"),
    attachments: [
      {
        id: "file",
        name: "file.txt",
        download_url: "file.txt",
        size_bytes: 1,
        media_type: "text/plain",
        kind: "file",
        sha256: "hash",
        created_at: "2026-10-21T00:00:00Z",
      },
    ],
  };
  const updated = { ...before, attachments: [] };
  const next = mergeConversationMessageSnapshot(data([updated]), "room", [before], [before]);
  expect(next?.rooms[0].messages[0].attachments).toEqual([]);
});
