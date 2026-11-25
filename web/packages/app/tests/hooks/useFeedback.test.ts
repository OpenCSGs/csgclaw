import { act, renderHook } from "@testing-library/react";
import { submitFeedback } from "@/api/feedback";
import { useFeedback } from "@/components/business/Feedback/useFeedback";

vi.mock("@/api/feedback", () => ({ submitFeedback: vi.fn() }));
beforeEach(() => vi.clearAllMocks());

it("preserves content after failure and allows an explicit retry", async () => {
  vi.mocked(submitFeedback)
    .mockRejectedValueOnce(new Error("Network disconnected"))
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
  expect(submitFeedback).toHaveBeenLastCalledWith("A problem", [image], []);
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

it("clears form state when the signed-in account or site changes", () => {
  const { result, rerender } = renderHook(({ scope }) => useFeedback((key) => key, scope), {
    initialProps: { scope: "user:site-a" },
  });
  act(() => result.current.changeDescription("Private feedback"));
  rerender({ scope: "user:site-b" });
  expect(result.current.description).toBe("");
});

it("keeps valid files in a mixed batch and sends archives separately", async () => {
  vi.mocked(submitFeedback).mockResolvedValue({ success: true, issue_id: 42 });
  const { result } = renderHook(() => useFeedback((key, params) => String(params?.files ?? key)));
  const screenshot = new File(["image"], "Screenshot.PNG");
  const archive = new File(["zip"], "diagnostics.zip");
  const invalid = new File(["text"], "notes.txt", { type: "text/plain" });
  const oversized = new File([new Uint8Array(10 * 1024 * 1024 + 1)], "oversized.zip");
  act(() => result.current.addAttachments([screenshot, invalid, oversized, archive]));
  expect(result.current.images).toEqual([screenshot]);
  expect(result.current.archives).toEqual([archive]);
  expect(result.current.error).toBe("notes.txt, oversized.zip");
  await act(async () => {
    await result.current.submit();
  });
  expect(submitFeedback).toHaveBeenCalledWith("", [screenshot], [archive]);
  expect(result.current.receipt?.issue_id).toBe(42);
});
