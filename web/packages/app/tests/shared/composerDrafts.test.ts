import { createAttachmentDrafts } from "@/models/attachments";
import {
  acknowledgeComposerDraftSaveFailure,
  composerDraftKey,
  composerDraftSaveFailed,
  discardAcknowledgedComposerDraft,
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

it.each([
  ["a newer draft", { text: "newer draft" }],
  ["a cleared draft", null],
])("does not overwrite %s from another tab on pagehide", (_description, replacement) => {
  const key = composerDraftKey("test-user", "cross-tab");
  const storage = vi.spyOn(Storage.prototype, "setItem").mockImplementationOnce(() => {
    throw new DOMException("Storage is full", "QuotaExceededError");
  });
  saveComposerDraft(key, { text: "older unsaved draft" }, []);
  flushComposerDraft(key);
  expect(composerDraftSaveFailed(key)).toBe(true);

  storage.mockRestore();
  const persisted = replacement === null ? null : JSON.stringify(replacement);
  localStorage.setItem(key, JSON.stringify({ text: "draft saved in another tab" }));
  if (persisted === null) localStorage.removeItem(key);
  else localStorage.setItem(key, persisted);
  try {
    window.dispatchEvent(new Event("pagehide"));
    expect(localStorage.getItem(key)).toBe(persisted);
    expect(composerDraftSaveFailed(key)).toBe(true);
    expect(getComposerDraft(key).content).toEqual({ text: "older unsaved draft" });
  } finally {
    saveComposerDraft(key, null, []);
  }
});

it("flushes a fresh edit on pagehide after the previous write fails", () => {
  vi.useFakeTimers();
  const key = composerDraftKey("test-user", "fresh-edit-on-exit");
  localStorage.setItem(key, JSON.stringify({ text: "previously saved" }));
  const storage = vi.spyOn(Storage.prototype, "setItem").mockImplementationOnce(() => {
    throw new DOMException("Storage is full", "QuotaExceededError");
  });
  saveComposerDraft(key, { text: "failed edit" }, []);
  vi.advanceTimersByTime(250);
  expect(composerDraftSaveFailed(key)).toBe(true);

  saveComposerDraft(key, { text: "fresh edit" }, []);
  expect(composerDraftSaveFailed(key)).toBe(true);
  expect(storage).toHaveBeenCalledTimes(1);
  window.dispatchEvent(new Event("pagehide"));
  vi.advanceTimersByTime(250);

  expect(storage).toHaveBeenCalledTimes(2);
  expect(composerDraftSaveFailed(key)).toBe(false);
  expect(JSON.parse(localStorage.getItem(key) ?? "{}")).toMatchObject({ text: "fresh edit" });
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
  const newerDraft = JSON.stringify({ text: "saved in another tab" });
  localStorage.setItem(key, newerDraft);
  window.dispatchEvent(new Event("pagehide"));
  expect(localStorage.getItem(key)).toBe(newerDraft);
  expect(getComposerDraft(key).content).toEqual({});
  expect(composerDraftSaveFailed(key)).toBe(true);

  flushComposerDraft(key);
  expect(composerDraftSaveFailed(key)).toBe(false);
  expect(localStorage.getItem(key)).toBeNull();
});

it("keeps acknowledged text in the current editor and discards it on leaving", () => {
  const key = composerDraftKey("test-user", "dismiss");
  localStorage.setItem(key, JSON.stringify({ text: "previously saved" }));
  const storage = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("Storage is full", "QuotaExceededError");
  });
  const leave = subscribeToComposerDraft(key, vi.fn());
  saveComposerDraft(key, { text: "unsaved edit" }, []);
  flushComposerDraft(key);
  acknowledgeComposerDraftSaveFailure(key);
  expect(composerDraftSaveFailed(key)).toBe(false);
  expect(getComposerDraft(key).content).toEqual({ text: "unsaved edit" });

  discardAcknowledgedComposerDraft(key);
  leave();
  expect(getComposerDraft(key).content).toEqual({});
  expect(localStorage.getItem(key)).toBeNull();
  expect(storage).toHaveBeenCalledOnce();
});

it.each(["route exit", "pagehide"])("preserves a newer draft from another tab on %s after dismissal", (exit) => {
  const key = composerDraftKey("test-user", `dismiss-${exit}`);
  localStorage.setItem(key, JSON.stringify({ text: "previously saved" }));
  const leave = subscribeToComposerDraft(key, vi.fn());
  expect(getComposerDraft(key).content).toEqual({ text: "previously saved" });
  const savedInOtherTab = JSON.stringify({ text: "newer draft in another tab" });
  if (exit === "route exit") localStorage.setItem(key, savedInOtherTab);

  vi.spyOn(Storage.prototype, "setItem").mockImplementationOnce(() => {
    throw new DOMException("Storage is full", "QuotaExceededError");
  });
  saveComposerDraft(key, { text: "unsaved edit" }, []);
  flushComposerDraft(key);
  acknowledgeComposerDraftSaveFailure(key);

  if (exit === "pagehide") localStorage.setItem(key, savedInOtherTab);
  if (exit === "pagehide") window.dispatchEvent(new Event("pagehide"));
  else discardAcknowledgedComposerDraft(key);

  expect(localStorage.getItem(key)).toBe(savedInOtherTab);
  expect(getComposerDraft(key).content).toEqual({ text: "newer draft in another tab" });
  expect(composerDraftSaveFailed(key)).toBe(false);
  leave();
});

it("saves new edits made after acknowledging a failure", () => {
  const key = composerDraftKey("test-user", "edit-after-dismiss");
  const storage = vi.spyOn(Storage.prototype, "setItem").mockImplementationOnce(() => {
    throw new DOMException("Storage is full", "QuotaExceededError");
  });
  saveComposerDraft(key, { text: "unsaved edit" }, []);
  flushComposerDraft(key);
  acknowledgeComposerDraftSaveFailure(key);
  saveComposerDraft(key, { text: "later edit" }, []);
  discardAcknowledgedComposerDraft(key);
  flushComposerDraft(key);

  expect(storage).toHaveBeenCalledTimes(2);
  expect(composerDraftSaveFailed(key)).toBe(false);
  expect(getComposerDraft(key).content).toEqual({ text: "later edit" });
});
