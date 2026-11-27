import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchDescriptionAccess } from "@/api/descriptionAccess";
import { useDescriptionAccess } from "./useDescriptionAccess";

vi.mock("@/api/descriptionAccess", () => ({ fetchDescriptionAccess: vi.fn() }));

it("allows a failed probe to be retried and distinguishes denial from an error", async () => {
  vi.mocked(fetchDescriptionAccess)
    .mockRejectedValueOnce(new Error("Service unavailable"))
    .mockResolvedValueOnce({ allowed: true })
    .mockResolvedValueOnce({ allowed: false });
  const { result } = renderHook(() => useDescriptionAccess(true));

  await waitFor(() => expect(result.current.status).toBe("error"));
  act(() => result.current.retry());
  expect(result.current.status).toBe("checking");
  await waitFor(() => expect(result.current.status).toBe("allowed"));
  act(() => result.current.retry());
  await waitFor(() => expect(result.current.status).toBe("denied"));
  expect(fetchDescriptionAccess).toHaveBeenCalledTimes(3);
});

it("aborts an unanswered probe and allows a successful retry", async () => {
  vi.useFakeTimers();
  let firstSignal: AbortSignal | undefined;
  vi.mocked(fetchDescriptionAccess)
    .mockImplementationOnce((signal) => {
      firstSignal = signal;
      return new Promise(() => {});
    })
    .mockResolvedValueOnce({ allowed: true });
  const { result, unmount } = renderHook(() => useDescriptionAccess(true));
  try {
    expect(result.current.status).toBe("checking");
    act(() => vi.advanceTimersByTime(5000));
    expect(firstSignal?.aborted).toBe(true);
    expect(result.current.status).toBe("error");
    await act(async () => result.current.retry());
    expect(result.current.status).toBe("allowed");
  } finally {
    unmount();
    vi.useRealTimers();
  }
});
