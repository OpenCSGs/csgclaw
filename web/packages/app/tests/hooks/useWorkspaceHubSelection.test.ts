import {
  resolveRouteDrivenHubSelection,
  resolveHubTemplateSelection,
  templateWorkspaceFilesStateNeedsReset,
} from "@/hooks/workspace/useWorkspaceHubSelection";
import { paneFromLocation } from "@/models/routing";

const storedHubSelection = {
  selectedHubResourceType: "mcp" as const,
  selectedHubSkillName: "first-skill",
  selectedHubTemplateId: "first-template",
  selectedKnowledgeBaseID: "first-knowledge-base",
  selectedMCPServerName: "first-mcp",
};

describe("resolveRouteDrivenHubSelection", () => {
  it.each([
    ["/templates/template-2", "template", "selectedHubTemplateId", "template-2"],
    ["/skills/agent-builder", "skill", "selectedHubSkillName", "agent-builder"],
    ["/mcp-servers/mcp-2", "mcp", "selectedMCPServerName", "mcp-2"],
    ["/knowledge-bases/42", "knowledge", "selectedKnowledgeBaseID", "42"],
  ] as const)("uses the addressed resource from %s", (path, resourceType, selectionKey, resourceID) => {
    const selection = resolveRouteDrivenHubSelection(paneFromLocation(path), storedHubSelection);

    expect(selection.selectedHubResourceType).toBe(resourceType);
    expect(selection[selectionKey]).toBe(resourceID);
  });

  it("keeps the in-memory selection on the resources landing route", () => {
    expect(resolveRouteDrivenHubSelection(paneFromLocation("/resources"), storedHubSelection)).toBe(storedHubSelection);
  });
});

describe("resolveHubTemplateSelection", () => {
  const templates = [{ id: "template-1" }, { id: "template-2" }] as Parameters<typeof resolveHubTemplateSelection>[0];

  it("keeps the list visible when no template is selected", () => {
    expect(resolveHubTemplateSelection(templates, "")).toBe("");
  });

  it("preserves a direct-detail selection only while it can still resolve", () => {
    expect(resolveHubTemplateSelection(templates, "pending-template", true)).toBe("pending-template");
    expect(resolveHubTemplateSelection([], "pending-template", true)).toBe("pending-template");
    expect(resolveHubTemplateSelection(templates, "missing-template")).toBe("");
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
