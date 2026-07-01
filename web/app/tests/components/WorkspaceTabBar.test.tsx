import { render, screen, within } from "@testing-library/react";
import { WorkspaceTabBar } from "@/pages/WorkspacePage/components/WorkspaceSidebar/WorkspaceTabBar";
import { WorkspaceTabs } from "@/models/routing";
import type { TranslateFn } from "@/models/conversations";

const labels: Record<string, string> = {
  agentsTab: "Agents",
  hubTab: "Hub",
  messagesTab: "Messages",
  newBadge: "NEW",
  tasksTab: "Tasks",
};

const t: TranslateFn = (key) => labels[key] ?? key;

describe("WorkspaceTabBar", () => {
  it("exposes the tasks shortcut in the sidebar rail", () => {
    render(
      <WorkspaceTabBar
        variant="rail"
        workspaceTab={WorkspaceTabs.messages}
        onWorkspaceTabChange={() => {}}
        roomCount={2}
        agentCount={1}
        taskCount={0}
        onSelectHub={() => {}}
        showHubNewBadge
        t={t}
      />,
    );

    const tablist = screen.getByRole("tablist", { name: "Workspace sections" });

    expect(within(tablist).getByRole("tab", { name: "Messages" })).toBeInTheDocument();
    expect(within(tablist).getByRole("tab", { name: "Agents" })).toBeInTheDocument();
    expect(within(tablist).getByRole("tab", { name: "Tasks" })).toBeInTheDocument();
    expect(within(tablist).getByRole("tab", { name: "Hub" })).toBeInTheDocument();
  });

  it("hides the hub new badge after it is dismissed", () => {
    render(
      <WorkspaceTabBar
        workspaceTab={WorkspaceTabs.messages}
        onWorkspaceTabChange={() => {}}
        roomCount={2}
        agentCount={1}
        taskCount={0}
        onSelectHub={() => {}}
        showHubNewBadge={false}
        t={t}
      />,
    );

    expect(screen.queryByText("NEW")).not.toBeInTheDocument();
  });
});
