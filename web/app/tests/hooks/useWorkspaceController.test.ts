import { describe, expect, it, vi } from "vitest";
import { saveMCPServerAndSelect } from "@/hooks/workspace/useWorkspaceController";

const payload = {
  config: { type: "remote", url: "https://gateway.example.test/mcp" },
  name: "agentichub-kb-42",
};

describe("saveMCPServerAndSelect", () => {
  it("selects the saved MCP server only after creation succeeds", async () => {
    const onSaved = vi.fn();

    await expect(saveMCPServerAndSelect(payload, async () => false, onSaved)).resolves.toBe(false);
    expect(onSaved).not.toHaveBeenCalled();

    await expect(saveMCPServerAndSelect(payload, async () => true, onSaved)).resolves.toBe(true);
    expect(onSaved).toHaveBeenCalledWith("agentichub-kb-42");
  });
});
