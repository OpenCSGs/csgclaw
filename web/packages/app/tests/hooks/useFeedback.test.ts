import { act, renderHook, waitFor } from "@testing-library/react";
import { readFeedbackStatus, submitFeedback } from "@/api/feedback";
import { useFeedback } from "@/components/business/Feedback/useFeedback";
import { readFeedbackDraft, saveFeedbackDraft } from "@/shared/storage/feedbackDraft";

vi.mock("@/api/feedback", () => ({ submitFeedback: vi.fn(), readFeedbackStatus: vi.fn() }));
vi.mock("@/shared/storage/feedbackDraft", () => ({ readFeedbackDraft: vi.fn(), saveFeedbackDraft: vi.fn() }));
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(readFeedbackDraft).mockResolvedValue(undefined);
  vi.mocked(saveFeedbackDraft).mockResolvedValue(undefined);
});

it("preserves content after failure and allows an explicit retry", async () => {
  vi.mocked(submitFeedback)
    .mockRejectedValueOnce({ code: "feedback_upload_failed" })
    .mockResolvedValueOnce({ success: true });
  const { result } = renderHook(() => useFeedback((key) => key));
  const image = new File(["image"], "test.png", { type: "image/png" });
  act(() => {
    result.current.changeDescription("A problem");
    result.current.addAttachments([image]);
  });
  await act(async () => {
    await result.current.submit();
  });
  expect(result.current.description).toBe("A problem");
  expect(result.current.images).toEqual([image]);
  expect(submitFeedback).toHaveBeenCalledOnce();
  await act(async () => {
    await result.current.submit();
  });
  const attempt = vi.mocked(submitFeedback).mock.calls[0][3];
  expect(submitFeedback).toHaveBeenLastCalledWith("A problem", [image], [], attempt);
  expect(result.current.receipt?.success).toBe(true);
  expect(result.current.description).toBe("");
});

it("prevents simultaneous submits", async () => {
  let resolve: (value: { success: boolean }) => void = () => {};
  vi.mocked(submitFeedback).mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const { result } = renderHook(() => useFeedback((key) => key));
  act(() => result.current.changeDescription("A problem"));
  let first: Promise<boolean>;
  act(() => {
    first = result.current.submit();
  });
  await act(async () => {
    await result.current.submit();
  });
  expect(submitFeedback).toHaveBeenCalledOnce();
  await act(async () => {
    resolve({ success: true });
    await first;
  });
});

it("clears form state when the signed-in account or site changes", async () => {
  const { result, rerender } = renderHook(({ scope }) => useFeedback((key) => key, scope), {
    initialProps: { scope: "user:site-a" },
  });
  await waitFor(() => expect(result.current.ready).toBe(true));
  await act(async () => result.current.changeDescription("Private feedback"));
  rerender({ scope: "user:site-b" });
  expect(result.current.description).toBe("");
  await waitFor(() => expect(result.current.ready).toBe(true));
});

it("keeps valid files in a mixed batch and sends archives separately", async () => {
  vi.mocked(submitFeedback).mockResolvedValue({ success: true, issue_id: 42 });
  const { result } = renderHook(() => useFeedback((key) => key));
  const screenshot = new File(["image"], "Screenshot.PNG");
  const archive = new File(["zip"], "diagnostics.zip");
  const invalid = new File(["text"], "notes.txt", { type: "text/plain" });
  const oversized = new File([new Uint8Array(10 * 1024 * 1024 + 1)], "oversized.zip");
  act(() => result.current.addAttachments([screenshot, invalid, oversized, archive]));
  expect(result.current.images).toEqual([screenshot]);
  expect(result.current.archives).toEqual([archive]);
  expect(result.current.rejections).toEqual([
    { file: "notes.txt", reason: "feedbackAttachmentFormat" },
    { file: "oversized.zip", reason: "feedbackArchiveTooLarge" },
  ]);
  act(() => result.current.removeAttachment("image", 0));
  expect(result.current.rejections).toHaveLength(2);
  act(() => result.current.addAttachments([screenshot]));
  await act(async () => {
    await result.current.submit();
  });
  expect(submitFeedback).toHaveBeenCalledWith("", [screenshot], [archive], expect.stringMatching(/^[a-f0-9]{32}$/));
  expect(result.current.receipt?.issue_id).toBe(42);
});

it("restores a pending draft and checks the receipt without resending", async () => {
  const image = new File(["image"], "feedback.png", { type: "image/png" });
  const requestID = "0123456789abcdef0123456789abcdef";
  vi.mocked(readFeedbackDraft).mockResolvedValue({
    description: "Saved draft",
    images: [image],
    archives: [],
    requestID,
    awaitingStatus: true,
  });
  vi.mocked(readFeedbackStatus).mockResolvedValue({ state: "submitted", receipt: { success: true, issue_id: 12 } });
  const { result } = renderHook(() => useFeedback((key) => key, "my-site:my-account"));
  await waitFor(() => expect(result.current.ready).toBe(true));
  expect(result.current.description).toBe("Saved draft");
  expect(result.current.images).toEqual([image]);
  await act(async () => {
    await result.current.submit();
  });
  expect(submitFeedback).not.toHaveBeenCalled();
  await act(async () => {
    await result.current.checkStatus();
  });
  expect(readFeedbackStatus).toHaveBeenCalledWith(requestID);
  expect(result.current.receipt?.issue_id).toBe(12);
  expect(saveFeedbackDraft).toHaveBeenLastCalledWith(
    "my-site:my-account",
    { description: "", images: [], archives: [] },
    true,
  );
});

it("checks an unknown result and reuses its reference when the attempt was not recorded", async () => {
  vi.mocked(submitFeedback)
    .mockRejectedValueOnce(new Error("Network disconnected"))
    .mockResolvedValueOnce({ success: true });
  vi.mocked(readFeedbackStatus)
    .mockResolvedValueOnce({ state: "unknown" })
    .mockResolvedValueOnce({ state: "not_found" });
  const { result } = renderHook(() => useFeedback((key) => key));
  act(() => result.current.changeDescription("A problem"));
  await act(async () => {
    await result.current.submit();
  });
  expect(result.current.awaitingStatus).toBe(true);
  await act(async () => {
    await result.current.checkStatus();
  });
  expect(result.current.awaitingStatus).toBe(true);
  await act(async () => {
    await result.current.submit();
  });
  expect(submitFeedback).toHaveBeenCalledOnce();
  await act(async () => {
    await result.current.checkStatus();
  });
  expect(result.current.awaitingStatus).toBe(false);
  await act(async () => {
    await result.current.submit();
  });
  expect(submitFeedback).toHaveBeenCalledTimes(2);
  expect(vi.mocked(submitFeedback).mock.calls[0][3]).toBe(vi.mocked(submitFeedback).mock.calls[1][3]);
});
