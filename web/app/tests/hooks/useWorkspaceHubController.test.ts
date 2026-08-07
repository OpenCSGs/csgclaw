import { visibleWorkspaceHubTemplates, workspaceHubMutationError } from "@/hooks/workspace/useWorkspaceHubController";
import type { HubTemplate } from "@/models/hubWorkspace";

describe("workspace hub controller errors", () => {
  it("only exposes mutation errors on their owning resource page", () => {
    expect(workspaceHubMutationError("template", "", "Publishing failed", "")).toBe("Publishing failed");
    expect(workspaceHubMutationError("skill", "", "Publishing failed", "Skill delete failed")).toBe(
      "Skill delete failed",
    );
    expect(workspaceHubMutationError("mcp", "Delete failed", "Publishing failed", "Skill delete failed")).toBe("");
  });

  it("hides a deleting template before the refreshed list arrives", () => {
    const templates: HubTemplate[] = [
      { id: "local.alpha", role: "worker", source: { kind: "local" } },
      { id: "local.beta", role: "worker", source: { kind: "local" } },
    ];

    expect(visibleWorkspaceHubTemplates(templates, "local.alpha").map((item) => item.id)).toEqual(["local.beta"]);
  });
});
