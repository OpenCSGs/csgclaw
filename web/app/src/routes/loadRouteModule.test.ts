import { describe, expect, it, vi } from "vitest";
import { isRouteChunkLoadError, loadRouteModule, type RouteChunkRecoveryEnvironment } from "./loadRouteModule";

function recoveryEnvironment(retryMarker: string | null = null): {
  environment: RouteChunkRecoveryEnvironment;
  reload: ReturnType<typeof vi.fn>;
  setRetryMarker: ReturnType<typeof vi.fn>;
} {
  const reload = vi.fn();
  const setRetryMarker = vi.fn();
  return {
    environment: {
      clearRetryMarker: vi.fn(),
      getRetryMarker: vi.fn(() => retryMarker),
      reload,
      setRetryMarker,
    },
    reload,
    setRetryMarker,
  };
}

describe("loadRouteModule", () => {
  it("reloads once when a stale route chunk cannot be imported", async () => {
    const error = new TypeError(
      "Failed to fetch dynamically imported module: http://127.0.0.1:18080/assets/ComputerPage-old.js",
    );
    const { environment, reload, setRetryMarker } = recoveryEnvironment();

    void loadRouteModule(() => Promise.reject(error), environment);

    await vi.waitFor(() => expect(reload).toHaveBeenCalledOnce());
    expect(setRetryMarker).toHaveBeenCalledWith(error.message);
  });

  it("surfaces the error instead of reloading repeatedly", async () => {
    const error = new TypeError(
      "Failed to fetch dynamically imported module: http://127.0.0.1:18080/assets/ComputerPage-old.js",
    );
    const { environment, reload } = recoveryEnvironment(error.message);

    await expect(loadRouteModule(() => Promise.reject(error), environment)).rejects.toBe(error);
    expect(reload).not.toHaveBeenCalled();
  });

  it("does not reload for route module evaluation errors", async () => {
    const error = new Error("ComputerPage initialization failed");
    const { environment, reload } = recoveryEnvironment();

    await expect(loadRouteModule(() => Promise.reject(error), environment)).rejects.toBe(error);
    expect(reload).not.toHaveBeenCalled();
  });

  it("clears a previous retry marker after a successful import", async () => {
    const { environment } = recoveryEnvironment("old failure");
    const loaded = { default: "ComputerPage" };

    await expect(loadRouteModule(() => Promise.resolve(loaded), environment)).resolves.toBe(loaded);
    expect(environment.clearRetryMarker).toHaveBeenCalledOnce();
  });
});

describe("isRouteChunkLoadError", () => {
  it.each([
    "Failed to fetch dynamically imported module: /assets/ComputerPage-old.js",
    "error loading dynamically imported module: /assets/ComputerPage-old.js",
    "Importing a module script failed.",
    "Failed to load module script: Expected a JavaScript module script",
    "Unable to preload CSS for /assets/ComputerPage-old.css",
  ])("recognizes browser chunk-load failure: %s", (message) => {
    expect(isRouteChunkLoadError(new TypeError(message))).toBe(true);
  });
});
