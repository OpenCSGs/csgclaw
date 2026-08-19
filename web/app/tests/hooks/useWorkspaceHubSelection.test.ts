import {
  resolveHubTemplateSelection,
  templateWorkspaceFilesStateNeedsReset,
} from "@/hooks/workspace/useWorkspaceHubSelection";

describe("resolveHubTemplateSelection", () => {
  const templates = [{ id: "template-1" }, { id: "template-2" }] as Parameters<typeof resolveHubTemplateSelection>[0];

  it("defaults an empty selection to the first listed template", () => {
    expect(resolveHubTemplateSelection(templates, "")).toBe("template-1");
  });

  it("preserves a direct-detail selection only while it can still resolve", () => {
    expect(resolveHubTemplateSelection(templates, "pending-template", true)).toBe("pending-template");
    expect(resolveHubTemplateSelection([], "pending-template", true)).toBe("pending-template");
    expect(resolveHubTemplateSelection(templates, "missing-template")).toBe("template-1");
    expect(resolveHubTemplateSelection([], "missing-template")).toBe("");
  });
});

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
