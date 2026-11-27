import { createAttachmentDrafts } from "@/models/attachments";
import {
  composerDraftKey,
  composerDraftSaveFailed,
  flushComposerDraft,
  getComposerDraft,
  saveComposerDraft,
  subscribeToComposerDraft,
} from "@/shared/storage/composerDrafts";
import { clearTestComposerDrafts } from "../helpers/composerDrafts";

afterEach(() => {
  vi.restoreAllMocks();
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

it("retains a failed write after leaving and retries only the latest draft", () => {
  vi.useFakeTimers();
  const key = composerDraftKey("test-user", "failed-write");
  const storage = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("Storage is full", "QuotaExceededError");
  });
  const leave = subscribeToComposerDraft(key, vi.fn());
  saveComposerDraft(key, { text: "first edit" }, []);
  const firstEdit = getComposerDraft(key);
  leave();
  vi.advanceTimersByTime(250);
  expect(composerDraftSaveFailed(key)).toBe(true);
  expect(getComposerDraft(key)).toBe(firstEdit);

  const listener = vi.fn();
  const leaveAgain = subscribeToComposerDraft(key, listener);
  saveComposerDraft(key, { text: "latest edit" }, []);
  const latestEdit = getComposerDraft(key);
  expect(composerDraftSaveFailed(key)).toBe(true);
  storage.mockRestore();
  listener.mockClear();
  flushComposerDraft(key);
  expect(composerDraftSaveFailed(key)).toBe(false);
  expect(listener).toHaveBeenCalledOnce();
  expect(getComposerDraft(key)).toBe(latestEdit);
  expect(JSON.parse(localStorage.getItem(key) ?? "{}")).toMatchObject({ text: "latest edit" });
  leaveAgain();
});

it("keeps an explicit clear when storage removal fails instead of restoring stale text", () => {
  const key = composerDraftKey("test-user", "failed-clear");
  localStorage.setItem(key, JSON.stringify({ text: "previously saved" }));
  const storage = vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
    throw new DOMException("Storage is blocked", "SecurityError");
  });
  const leave = subscribeToComposerDraft(key, vi.fn());
  saveComposerDraft(key, null, []);
  expect(composerDraftSaveFailed(key)).toBe(true);
  leave();
  expect(getComposerDraft(key).content).toEqual({});

  storage.mockRestore();
  window.dispatchEvent(new Event("pagehide"));
  expect(composerDraftSaveFailed(key)).toBe(false);
  expect(localStorage.getItem(key)).toBeNull();
});
