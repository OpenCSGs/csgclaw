import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TeamDetailPane } from "@/pages/TeamPage/components";
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
