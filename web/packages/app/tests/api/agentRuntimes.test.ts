import { get, post } from "@/api/client";
import { fetchAgentRuntimes, installAgentRuntime } from "@/api/agentRuntimes";

vi.mock("@/api/client", () => ({ get: vi.fn(), post: vi.fn() }));

describe("agent runtime installation API", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.mocked(get).mockReset();
    vi.mocked(post).mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("loads bundled runtime status without browser caching", async () => {
    vi.mocked(get).mockResolvedValue([]);

    await fetchAgentRuntimes();

    expect(get).toHaveBeenCalledWith("api/v1/agent-runtimes", { cache: "no-store" });
  });

  it("polls real installation progress until the runtime is ready", async () => {
    const onProgress = vi.fn();
    const runtime = {
      name: "dsh",
      installed: true,
      installable: true,
      status: "installed",
      path: "/home/test/.local/bin/dsh",
    };
    vi.mocked(post).mockResolvedValue({
      name: "dsh",
      status: "running",
      stage: "installing_packages",
      activity_count: 10,
      started_at: "2026-09-21T08:45:50Z",
    });
    vi.mocked(get).mockResolvedValue({
      name: "dsh",
      status: "succeeded",
      stage: "completed",
      activity_count: 1085,
      started_at: "2026-09-21T08:45:50Z",
      runtime,
    });

    const result = installAgentRuntime("dsh", onProgress);
    await vi.advanceTimersByTimeAsync(500);

    await expect(result).resolves.toEqual(runtime);
    expect(post).toHaveBeenCalledWith("api/v1/agent-runtimes/dsh/install");
    expect(get).toHaveBeenCalledWith("api/v1/agent-runtimes/dsh/install", { cache: "no-store" });
    expect(onProgress).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ status: "running", stage: "installing_packages", activityCount: 10 }),
    );
    expect(onProgress).toHaveBeenLastCalledWith(
      expect.objectContaining({ status: "succeeded", stage: "completed", activityCount: 1085 }),
    );
  });

  it("turns a failed background installation into a localized API error", async () => {
    vi.mocked(post).mockResolvedValue({
      name: "dsh",
      status: "failed",
      stage: "checking_environment",
      error_code: "dsh_node_required",
      message: "Node.js is required",
    });

    await expect(installAgentRuntime("dsh")).rejects.toMatchObject({
      code: "dsh_node_required",
      message: "Node.js is required",
    });
  });
});
