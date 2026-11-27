import { createAttachmentDrafts } from "@/models/attachments";
import { composerDraftKey, getComposerDraft, saveComposerDraft } from "@/shared/storage/composerDrafts";
import { clearTestComposerDrafts } from "../helpers/composerDrafts";

afterEach(() => {
  clearTestComposerDrafts();
  vi.useRealTimers();
});

it("debounces writes and persists only attachment metadata", () => {
  vi.useFakeTimers();
  const key = composerDraftKey("test-user", "room");
  const [attachment] = createAttachmentDrafts([new File(["contents"], "example.txt", { type: "text/plain" })]);
  saveComposerDraft(key, { text: "old" }, [attachment]);
  vi.advanceTimersByTime(200);
  saveComposerDraft(key, { text: "latest" }, [attachment]);
  vi.advanceTimersByTime(200);
  expect(localStorage.getItem(key)).toBeNull();

  vi.advanceTimersByTime(100);
  const { file, ...meta } = attachment;
  expect(file).toBeInstanceOf(File);
  expect(JSON.parse(localStorage.getItem(key) ?? "{}")).toEqual({ text: "latest", attachments: [meta] });
});

it("cancels the pending write when clearing a draft", () => {
  vi.useFakeTimers();
  const key = composerDraftKey("test-user", "room");
  saveComposerDraft(key, { text: "unsent" }, []);
  saveComposerDraft(key, null, []);
  vi.runAllTimers();
  window.dispatchEvent(new Event("pagehide"));
  expect(localStorage.getItem(key)).toBeNull();
});

it("ignores malformed data and scopes storage by user and room", () => {
  const key = composerDraftKey("other-user", "room");
  localStorage.setItem(key, "{");
  expect(getComposerDraft(key)).toEqual({ content: {}, attachments: [] });
  expect(new Set([key, composerDraftKey("test-user", "room"), composerDraftKey("test-user")]).size).toBe(3);
});
