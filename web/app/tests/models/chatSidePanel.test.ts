import { describe, expect, it } from "vitest";
import {
  closeChatSideTab,
  replaceChatSideTab,
  initialChatSideState,
  openChatSideTab,
  restoreChatSideState,
  serializeChatSideState,
} from "@/models/chatSidePanel";
import {
  composerFileReferences,
  normalizeTextMentions,
  parseComposerSegments,
  renderComposerSegments,
  serializeComposerSegments,
  type ComposerSegment,
} from "@/models/composer";
import { normalizeBrowserURL } from "@/models/fileReferences";

const reference = {
  source: "workspace" as const,
  agent_id: "alice",
  path: "docs/合同 draft.md",
  label: "docs/合同 draft.md",
  name: "合同 draft.md",
};

describe("chat sidebar state", () => {
  it("deduplicates by identity and activates a neighbor when closing", () => {
    let state = openChatSideTab(initialChatSideState(), { id: "files", kind: "files" });
    state = openChatSideTab(state, {
      id: "a",
      kind: "file",
      name: reference.name,
      reference,
      sizeBytes: 12,
      mediaType: "",
    });
    state = openChatSideTab(state, { id: "files", kind: "files" });
    expect(state.tabs).toHaveLength(2);
    expect(state.activeID).toBe("files");
    state = closeChatSideTab(state, "files");
    expect(state.activeID).toBe("a");
    state = closeChatSideTab(state, "a");
    expect(state.tabs).toEqual([]);
    expect(state.open).toBe(true);
  });
  it("returns a file opened from the directory to Files instead of a neighboring browser tab", () => {
    let state = openChatSideTab(initialChatSideState(), { id: "files", kind: "files" });
    state = openChatSideTab(state, { id: "browser", kind: "browser", url: "https://example.org/" });
    state = openChatSideTab(state, {
      id: "readme",
      kind: "file",
      name: reference.name,
      reference,
      sizeBytes: 12,
      mediaType: "",
      originTabID: "files",
    });
    expect(restoreChatSideState(serializeChatSideState(state)).tabs[2]).toEqual(state.tabs[2]);
    expect(closeChatSideTab(state, "readme").activeID).toBe("files");
    expect(closeChatSideTab(closeChatSideTab(state, "files"), "readme").activeID).toBe("browser");
    state = openChatSideTab(state, {
      id: "readme",
      kind: "file",
      name: reference.name,
      reference,
      sizeBytes: 12,
      mediaType: "",
    });
    expect(closeChatSideTab(state, "readme").activeID).toBe("browser");
  });
  it("restores new tabs and chooses a tool in place without duplicating singleton tabs", () => {
    let state = openChatSideTab(initialChatSideState(), { id: "files", kind: "files" });
    state = openChatSideTab(state, { id: "new-1", kind: "new" });
    expect(restoreChatSideState(serializeChatSideState(state))).toEqual(state);
    state = replaceChatSideTab(state, "new-1", { id: "files", kind: "files" });
    expect(state.tabs).toEqual([{ id: "files", kind: "files" }]);
    expect(state.activeID).toBe("files");
    state = openChatSideTab(state, { id: "new-2", kind: "new" });
    state = replaceChatSideTab(state, "new-2", { id: "new-2", kind: "browser", url: "" });
    expect(state.tabs.map((tab) => tab.kind)).toEqual(["files", "browser"]);
    expect(state.activeID).toBe("new-2");
  });
  it("restores source identities without serializing draft files or download URLs", () => {
    let state = openChatSideTab(initialChatSideState(), {
      id: "file",
      kind: "file",
      name: reference.name,
      reference,
      sizeBytes: 12,
      mediaType: "",
    });
    state = openChatSideTab(state, {
      id: "draft",
      kind: "file",
      name: "draft.md",
      sizeBytes: 5,
      mediaType: "",
      draft: {
        id: "draft",
        name: "draft.md",
        sizeBytes: 5,
        mediaType: "",
        previewURL: "blob:temporary",
        file: new File(["private"], "draft.md"),
      },
    });
    const stored = serializeChatSideState(state);
    expect(stored).not.toContain("private");
    expect(stored).not.toContain("blob:");
    const restored = restoreChatSideState(stored);
    expect(restored.tabs).toHaveLength(1);
    expect(restored.activeID).toBe("file");
    expect(restoreChatSideState("bad JSON")).toEqual(initialChatSideState());
    expect(
      restoreChatSideState(JSON.stringify({ tabs: [{ id: "bad", kind: "browser", url: "javascript:alert(1)" }] })).tabs,
    ).toEqual([]);
  });
});

describe("file references", () => {
  it("round-trips tokens with spaces and Unicode without creating person mentions", () => {
    const editor = document.createElement("div");
    const segments: ComposerSegment[] = [
      { type: "text", text: "Review " },
      { type: "file", reference, text: `@${reference.label}` },
    ];
    renderComposerSegments(editor, segments);
    const parsed = parseComposerSegments(editor);
    expect(parsed).toEqual(segments);
    expect(normalizeTextMentions(parsed, new Map([["docs", { id: "user-docs", name: "docs" }]]))).toEqual(segments);
    expect(serializeComposerSegments(parsed)).toBe('Review @"docs/合同 draft.md"');
    expect(composerFileReferences([...parsed, ...parsed])).toEqual([
      { source: "workspace", agent_id: "alice", path: reference.path },
    ]);
  });
  it("accepts only credential-free HTTP and HTTPS browser addresses", () => {
    expect(normalizeBrowserURL(" https://example.org/a ")).toBe("https://example.org/a");
    for (const [address, expected] of [
      ["qq.com", "https://qq.com/"],
      ["example.org/path?q=abc", "https://example.org/path?q=abc"],
      ["//example.org", "https://example.org/"],
      ["example.org:8443/path", "https://example.org:8443/path"],
      ["localhost:8080", "http://localhost:8080/"],
      ["127.0.0.1:8080", "http://127.0.0.1:8080/"],
      ["[::1]:8080", "http://[::1]:8080/"],
      ["http://example.org", "http://example.org/"],
      ["例子.中国", "https://xn--fsqu00a.xn--fiqs8s/"],
    ])
      expect(normalizeBrowserURL(address)).toBe(expected);
    expect(
      restoreChatSideState(JSON.stringify({ tabs: [{ id: "browser", kind: "browser", url: "qq.com" }] })).tabs,
    ).toEqual([{ id: "browser", kind: "browser", url: "https://qq.com/" }]);

    for (const url of [
      "javascript:alert(1)",
      "ftp://example.org",
      "/etc/passwd",
      "https://exa\nmple.org",
      "example.org:99999",
      "not an address",
      "user:secret@example.org",
      "file:///etc/passwd",
      "data:text/html,hi",
      "https://user:secret@example.org/",
    ])
      expect(normalizeBrowserURL(url)).toBeNull();
  });
});
