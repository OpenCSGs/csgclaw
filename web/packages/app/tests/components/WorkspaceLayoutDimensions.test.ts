import {
  SidebarWidth,
  clampPrimarySidebarWidth,
  normalizeStoredPrimarySidebarWidth,
  workspacePrimarySidebarLabels,
  workspacePrimarySidebarWidth,
  workspacePrimarySidebarWidthBounds,
  workspaceSidebarWidthBounds,
} from "@/pages/WorkspacePage/components/WorkspaceLayout/sidebarDimensions";
import type { TranslateFn } from "@/models/conversations";

describe("workspace sidebar dimensions", () => {
  it("uses a middle primary sidebar width for the current Chinese navigation labels", () => {
    const labels: Record<string, string> = {
      agentsTab: "智能体",
      computerAgentsSection: "智能体",
      computersSection: "电脑",
      humanSection: "人类",
      messagesTab: "消息",
      notificationsSection: "通知",
      resourcesModelProvidersSection: "模型提供方",
      resourcesSkillsLabel: "技能",
      resourcesTab: "资源",
      resourcesTemplatesSection: "模板",
      scheduledTasksTab: "定时任务",
      tasksTab: "任务",
      teamsSection: "团队",
    };
    const t: TranslateFn = (key) => labels[key] ?? key;

    expect(workspacePrimarySidebarWidth(workspacePrimarySidebarLabels(t))).toBe(240);
  });

  it("grows only when the widest navigation label needs more room", () => {
    const currentWidth = workspacePrimarySidebarWidth([{ label: "模型提供方" }]);
    const widerWidth = workspacePrimarySidebarWidth([{ label: "很长很长很长很长很长的菜单" }]);

    expect(currentWidth).toBe(SidebarWidth.primaryFallback);
    expect(widerWidth).toBeGreaterThan(currentWidth);
    expect(widerWidth).toBeLessThanOrEqual(SidebarWidth.primaryMax);
    expect(workspaceSidebarWidthBounds().min).toBe(SidebarWidth.min);
  });

  it("keeps user-resizable primary sidebar widths inside the allowed range", () => {
    expect(workspacePrimarySidebarWidthBounds()).toEqual({ default: 240, max: 300, min: 200 });
    expect(clampPrimarySidebarWidth(180)).toBe(200);
    expect(clampPrimarySidebarWidth(256)).toBe(256);
    expect(clampPrimarySidebarWidth(340)).toBe(300);
  });

  it("normalizes persisted primary sidebar widths from localStorage", () => {
    expect(normalizeStoredPrimarySidebarWidth(null)).toBeNull();
    expect(normalizeStoredPrimarySidebarWidth("not-a-number")).toBeNull();
    expect(normalizeStoredPrimarySidebarWidth("180")).toBe(200);
    expect(normalizeStoredPrimarySidebarWidth("218")).toBe(218);
    expect(normalizeStoredPrimarySidebarWidth("268")).toBe(268);
    expect(normalizeStoredPrimarySidebarWidth("320")).toBe(300);
  });
});
