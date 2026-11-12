import type { useWorkspaceHubController } from "../useWorkspaceHubController";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { SkillSummary } from "@/models/skillhub";
import type { MCPServer, MCPServerPayload } from "@/models/mcp";
import type { RemoteKnowledgeBase } from "@/models/knowledgeBases";

export function bindWorkspaceHub({
  hub,
  onSelectTemplate,
  onSelectSkill,
  onSelectMCP,
  onSelectKnowledgeBase,
  onCreateMCP,
  onOpenSkillUpload,
  onRequireAuthentication,
}: {
  hub: ReturnType<typeof useWorkspaceHubController>["hub"];
  onSelectTemplate: (item: HubTemplate | null | undefined) => void;
  onSelectSkill: (item: SkillSummary | null | undefined) => void;
  onSelectMCP: (item: MCPServer | null | undefined) => void;
  onSelectKnowledgeBase: (item: RemoteKnowledgeBase | null | undefined) => void;
  onCreateMCP: (payload: MCPServerPayload) => Promise<boolean>;
  onOpenSkillUpload: () => void;
  onRequireAuthentication: () => boolean;
}) {
  return {
    ...hub,
    detailPaneProps: {
      ...hub.detailPaneProps,
      onSelectSkillInPlace: hub.setSelectedHubSkillName,
      onSelectMCPInPlace: hub.setSelectedMCPServerName,
      onSelectKnowledgeBaseInPlace: (item: RemoteKnowledgeBase | null | undefined) =>
        hub.setSelectedKnowledgeBaseID(item?.id || ""),
      onCreateMCPInPlace: (payload: MCPServerPayload) =>
        hub.detailPaneProps.onCreateMCP(payload, { preserveSelection: true }),
      onUpdateMCPInPlace: (name: string, payload: MCPServerPayload) =>
        hub.detailPaneProps.onUpdateMCP(name, payload, { preserveSelection: true }),
      onSelectTemplate,
      onSelectSkill: (name: string | null | undefined) => onSelectSkill(name ? { name, description: "" } : null),
      onSelectMCP: (name: string | null | undefined) => onSelectMCP(name ? { name, config: {} } : null),
      onSelectKnowledgeBase,
      onCreateMCP,
      onOpenSkillUpload,
      onKnowledgeBaseLogin: () => {
        onRequireAuthentication();
      },
    },
  };
}

export async function saveMCPServerAndSelect(
  payload: MCPServerPayload,
  onCreate: ((payload: MCPServerPayload) => Promise<boolean> | boolean) | undefined,
  onSaved: (name: string) => void,
): Promise<boolean> {
  const saved = await onCreate?.(payload);
  if (!saved) {
    return false;
  }
  onSaved(payload.name);
  return true;
}
