import { act, renderHook } from "@testing-library/react";
import { submitFeedback } from "@/api/feedback";
import { useFeedback } from "@/pages/SettingsPage/useFeedback";

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
    result.current.changeImages([image]);
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
  expect(submitFeedback).toHaveBeenLastCalledWith("A problem", [image]);
  expect(result.current.success).toBe(true);
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
