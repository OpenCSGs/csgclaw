import { act, render, screen } from "@testing-library/react";
import { ResourceLoadMore } from "@/components/business/AgentDetails/ResourceLoadMore";
import type { ResourceContinuation } from "@/hooks/workspace/useInfiniteAgentResources";

it("observes the profile scroll region and only loads eligible batches", () => {
  const callbacks: IntersectionObserverCallback[] = [];
  const observe = vi.fn();
  const disconnect = vi.fn();
  const observer = vi.fn(function (callback: IntersectionObserverCallback) {
    callbacks.push(callback);
    return { observe, disconnect };
  });
  vi.stubGlobal("IntersectionObserver", observer);
  const state: ResourceContinuation = {
    total: 40,
    hasMore: true,
    loading: false,
    failed: false,
    loadMore: vi.fn().mockResolvedValue(true),
    retry: vi.fn(),
  };
  const view = (continuation: ResourceContinuation) => (
    <div className="agent-profile-scroll-region">
      <ResourceLoadMore continuation={continuation} t={(key) => key} />
    </div>
  );
  const { rerender, unmount } = render(view(state));
  expect(observer).toHaveBeenCalledWith(expect.any(Function), {
    root: document.querySelector(".agent-profile-scroll-region"),
    rootMargin: "200px",
  });
  act(() => callbacks[0]([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver));
  expect(state.loadMore).toHaveBeenCalledTimes(1);
  rerender(view({ ...state, loading: true }));
  expect(disconnect).toHaveBeenCalled();
  expect(observer).toHaveBeenCalledTimes(1);
  rerender(view({ ...state, failed: true }));
  expect(screen.getByRole("button", { name: "retry" })).toBeInTheDocument();
  expect(observer).toHaveBeenCalledTimes(1);
  rerender(view({ ...state, hasMore: false }));
  expect(screen.getByRole("status")).toHaveTextContent("resourceAllLoaded");
  unmount();
  vi.unstubAllGlobals();
});
