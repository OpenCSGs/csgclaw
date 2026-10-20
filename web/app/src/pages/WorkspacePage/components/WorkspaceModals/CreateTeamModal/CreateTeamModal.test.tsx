import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { CreateTeamModal } from "./CreateTeamModal";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key) => key;

describe("CreateTeamModal", () => {
  it("forwards Windows and IME-compatible team name change events", () => {
    const onTeamTitleChange = vi.fn();
    render(
      <CreateTeamModal
        candidates={[]}
        onClose={() => {}}
        onCreate={async () => {}}
        onTeamMemberIDsChange={() => {}}
        onTeamTitleChange={onTeamTitleChange}
        submitError=""
        t={t}
        teamActionBusy={false}
        teamMemberIDs={[]}
        teamTitle=""
      />,
    );

    fireEvent.change(screen.getByPlaceholderText("teamNamePlaceholder"), {
      target: { value: "Windows 测试团队" },
    });

    expect(onTeamTitleChange).toHaveBeenCalledWith("Windows 测试团队");
  });

  it("uses a named modal, focuses the name, traps keyboard focus and closes with Escape", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <CreateTeamModal
        candidates={[{ id: "worker", name: "Worker", role: "worker" }]}
        teamTitle="Team"
        onTeamTitleChange={vi.fn()}
        teamMemberIDs={["worker"]}
        onTeamMemberIDsChange={vi.fn()}
        submitError=""
        teamActionBusy={false}
        onClose={onClose}
        onCreate={vi.fn()}
        t={t}
      />,
    );
    const dialog = screen.getByRole("dialog", { name: "teamCreate" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    await waitFor(() => expect(screen.getByRole("textbox", { name: "teamNameLabel" })).toHaveFocus());
    within(dialog).getByRole("button", { name: "teamCreate" }).focus();
    await user.tab();
    expect(within(dialog).getByRole("button", { name: "close" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("textbox", { name: "teamNameLabel" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("keeps a pending team save open and prevents changes", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <CreateTeamModal
        candidates={[{ id: "worker", name: "Worker", role: "worker" }]}
        teamTitle="Team"
        onTeamTitleChange={vi.fn()}
        teamMemberIDs={["worker"]}
        onTeamMemberIDsChange={vi.fn()}
        submitError=""
        teamActionBusy
        onClose={onClose}
        onCreate={vi.fn()}
        t={t}
      />,
    );
    expect(screen.getByRole("textbox", { name: "teamNameLabel" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "cancel" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "close" })).toBeDisabled();
    expect(screen.getAllByRole("checkbox").every((checkbox) => checkbox.hasAttribute("disabled"))).toBe(true);
    await user.keyboard("{Escape}");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("keeps locked team members selected when toggling all members", () => {
    function Harness() {
      const [teamMemberIDs, setTeamMemberIDs] = useState(["agent-manager"]);
      return (
        <CreateTeamModal
          candidates={[
            { id: "agent-manager", name: "Manager", role: "manager" },
            { id: "agent-worker", name: "Worker", role: "worker" },
          ]}
          lockedTeamMemberIDs={["agent-manager"]}
          onClose={() => {}}
          onCreate={async () => {}}
          onTeamMemberIDsChange={setTeamMemberIDs}
          onTeamTitleChange={() => {}}
          submitError=""
          t={t}
          teamActionBusy={false}
          teamMemberIDs={teamMemberIDs}
          teamTitle="Team"
        />
      );
    }

    render(<Harness />);
    const [allMembers, manager, worker] = screen.getAllByRole("checkbox");

    expect(manager).toBeChecked();
    expect(manager).toBeDisabled();
    expect(worker).not.toBeChecked();

    fireEvent.click(allMembers);
    expect(worker).toBeChecked();

    fireEvent.click(allMembers);
    expect(manager).toBeChecked();
    expect(worker).not.toBeChecked();
  });
});
