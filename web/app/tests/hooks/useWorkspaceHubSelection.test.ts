import { templateWorkspaceFilesStateNeedsReset } from "@/hooks/workspace/useWorkspaceHubSelection";

describe("templateWorkspaceFilesStateNeedsReset", () => {
  it("keeps an already empty state stable", () => {
    expect(templateWorkspaceFilesStateNeedsReset({ files: {}, templateID: "" })).toBe(false);
  });

  it("resets stale template or file state", () => {
    expect(templateWorkspaceFilesStateNeedsReset({ files: {}, templateID: "template-1" })).toBe(true);
    expect(
      templateWorkspaceFilesStateNeedsReset({
        files: {
          "instructions/AGENTS.md": {
            content: "# Agent",
            path: "instructions/AGENTS.md",
          },
        },
        templateID: "",
      }),
    ).toBe(true);
  });
});
