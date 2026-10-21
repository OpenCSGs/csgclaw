import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TeamDetailPane } from "./TeamDetailPane";
import type { TranslateFn } from "@/models/conversations";
import type { WorkspaceTeam } from "@/models/tasks";

const t: TranslateFn = (key) => key;

const team: WorkspaceTeam = {
  id: "team-1",
  title: "Windows team",
  lead_agent_id: "u-manager",
  member_agent_ids: [],
  status: "active",
  created_at: "2026-08-01T00:00:00Z",
  updated_at: "2026-08-01T00:00:00Z",
};

describe("TeamDetailPane", () => {
  it("saves and clears descriptions on blur and cancels with Escape", async () => {
    const user = userEvent.setup();
    const onUpdateDescription = vi.fn().mockResolvedValue(true);
    render(<TeamDetailPane team={team} t={t} onUpdateDescription={onUpdateDescription} />);
    await user.click(screen.getByRole("button", { name: "editDescription" }));
    const input = screen.getByRole("textbox", { name: "teamDescriptionLabel" });
    await user.type(input, "Team introduction");
    await user.tab();
    expect(onUpdateDescription).toHaveBeenCalledWith(team, "Team introduction");
    await user.click(screen.getByRole("button", { name: "editDescription" }));
    await user.clear(screen.getByRole("textbox", { name: "teamDescriptionLabel" }));
    await user.keyboard("{Escape}");
    expect(onUpdateDescription).toHaveBeenCalledTimes(1);
  });
  it("shows only the saved team description and leaves missing descriptions blank", () => {
    const view = render(<TeamDetailPane team={{ ...team, description: "Review releases together." }} t={t} />);
    expect(screen.getByText("Review releases together.")).toBeVisible();

    view.rerender(<TeamDetailPane team={{ ...team, description: "  " }} t={t} />);
    expect(screen.queryByText("Review releases together.")).not.toBeInTheDocument();
    expect(screen.queryByText("teamDetailSubtitle")).not.toBeInTheDocument();

    view.rerender(<TeamDetailPane team={team} t={t} />);
    expect(screen.queryByText("teamDetailSubtitle")).not.toBeInTheDocument();
  });
  it("starts collaboration with the current team", async () => {
    const onStartCollaboration = vi.fn();
    render(<TeamDetailPane team={team} t={t} onStartCollaboration={onStartCollaboration} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "teamStartCollaboration" }));
    expect(onStartCollaboration).toHaveBeenCalledExactlyOnceWith(team);
  });
  it("confirms team deletion in-app without opening a native browser dialog", async () => {
    const user = userEvent.setup();
    const onDeleteTeam = vi.fn().mockResolvedValue(true);
    const confirmSpy = vi.spyOn(window, "confirm");

    try {
      render(<TeamDetailPane team={team} t={t} onDeleteTeam={onDeleteTeam} />);

      await user.click(screen.getByRole("button", { name: "teamDelete" }));

      const dialog = screen.getByRole("dialog", { name: "teamDelete" });
      expect(within(dialog).getByText("teamDeleteConfirm")).toBeInTheDocument();
      expect(onDeleteTeam).not.toHaveBeenCalled();
      expect(confirmSpy).not.toHaveBeenCalled();

      await user.click(within(dialog).getByRole("button", { name: "teamDelete" }));

      await waitFor(() => expect(onDeleteTeam).toHaveBeenCalledWith(team));
      expect(confirmSpy).not.toHaveBeenCalled();
    } finally {
      confirmSpy.mockRestore();
    }
  });
});
