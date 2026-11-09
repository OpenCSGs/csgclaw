import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { fetchRelayCatalog } from "@/api/relay";
import { emptyAuthStatus } from "@/models/auth";
import { RELAY_PUBLICATIONS_CHANGED_EVENT } from "@/shared/constants/relay";
import { cloudAgentFixture } from "../../../tests/helpers/cloudAgentFixture";
import { createQueryWrapper } from "../../../tests/helpers/queryClient";
import { cloudAgentCatalogKey, useCloudAgentCatalog } from "./useCloudAgentCatalog";

vi.mock("@/api/relay", () => ({ fetchRelayCatalog: vi.fn() }));
const auth = { ...emptyAuthStatus(), authenticated: true, base_url: "https://community.test", user_id: "alice" };

beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(fetchRelayCatalog)
    .mockReset()
    .mockResolvedValue([...cloudAgentFixture]);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

async function advance(milliseconds = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(milliseconds);
  });
}

it("never fetches for passive consumers, including focus and publication events", async () => {
  const { wrapper, client } = createQueryWrapper();
  const { result } = renderHook(() => useCloudAgentCatalog(auth, false), { wrapper });
  act(() => {
    window.dispatchEvent(new Event("focus"));
    window.dispatchEvent(new Event(RELAY_PUBLICATIONS_CHANGED_EVENT));
    client.setQueryData(cloudAgentCatalogKey(auth), [...cloudAgentFixture]);
  });
  await advance(15000);
  expect(result.current.data).toEqual(cloudAgentFixture);
  expect(fetchRelayCatalog).not.toHaveBeenCalled();
});

it("shares one refresh timer for staggered active consumers and stops when only passive readers remain", async () => {
  const { wrapper } = createQueryWrapper();
  const passive = renderHook(() => useCloudAgentCatalog(auth, false), { wrapper });
  const first = renderHook(() => useCloudAgentCatalog(auth), { wrapper });
  await advance();
  expect(fetchRelayCatalog).toHaveBeenCalledTimes(1);
  expect(passive.result.current.data).toEqual(cloudAgentFixture);
  await advance(2000);
  const second = renderHook(() => useCloudAgentCatalog(auth), { wrapper });
  await advance();
  expect(fetchRelayCatalog).toHaveBeenCalledTimes(1);
  await advance(3000);
  expect(fetchRelayCatalog).toHaveBeenCalledTimes(2);
  await advance(2000);
  expect(fetchRelayCatalog).toHaveBeenCalledTimes(2);
  first.unmount();
  await advance(3000);
  expect(fetchRelayCatalog).toHaveBeenCalledTimes(3);
  second.unmount();
  act(() => window.dispatchEvent(new Event("focus")));
  await advance(15000);
  expect(fetchRelayCatalog).toHaveBeenCalledTimes(3);
  expect(passive.result.current.data).toEqual(cloudAgentFixture);
});

it("shares an in-flight refresh across focus, publication events, and timer ticks", async () => {
  const { wrapper } = createQueryWrapper();
  const active = renderHook(() => useCloudAgentCatalog(auth), { wrapper });
  const passive = renderHook(() => useCloudAgentCatalog(auth, false), { wrapper });
  await advance();
  const updated = [{ ...cloudAgentFixture[0], name: "Updated" }];
  let resolve!: (value: typeof updated) => void;
  vi.mocked(fetchRelayCatalog).mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  act(() => window.dispatchEvent(new Event("focus")));
  await advance();
  act(() => window.dispatchEvent(new Event(RELAY_PUBLICATIONS_CHANGED_EVENT)));
  await advance(10000);
  expect(fetchRelayCatalog).toHaveBeenCalledTimes(2);
  expect(vi.mocked(fetchRelayCatalog).mock.calls[1][0]?.aborted).toBe(false);
  act(() => resolve(updated));
  await advance();
  expect(active.result.current.data).toEqual(updated);
  expect(passive.result.current.data).toEqual(updated);
});

it.each([
  { user_id: "bob" },
  { base_url: "https://other.test" },
  { logged_in_at: "new-session" },
  { authenticated: false },
])("does not expose a different identity's cached catalog: %j", async (change) => {
  const { wrapper, client } = createQueryWrapper();
  client.setQueryData(cloudAgentCatalogKey(auth), [...cloudAgentFixture]);
  const { result, rerender } = renderHook(({ identity }) => useCloudAgentCatalog(identity, false), {
    wrapper,
    initialProps: { identity: auth },
  });
  expect(result.current.data).toEqual(cloudAgentFixture);
  rerender({ identity: { ...auth, ...change } });
  await advance();
  expect(result.current.data).toBeUndefined();
  expect(fetchRelayCatalog).not.toHaveBeenCalled();
});
