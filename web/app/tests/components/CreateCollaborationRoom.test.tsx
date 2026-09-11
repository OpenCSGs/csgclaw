import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CreateRoomModal } from "@/pages/WorkspacePage/components/WorkspaceModals/CreateRoomModal";

describe("collaboration room creation", () => {
  it("explains the mode and prevents creation without a manager", async () => {
    const create = vi.fn();
    const changeMode = vi.fn();
    render(
      <CreateRoomModal
        t={(key) => key}
        candidates={[]}
        lockedRoomMemberIDs={[]}
        roomMemberIDs={[]}
        roomTitle="Build"
        roomDescription=""
        onRoomTitleChange={vi.fn()}
        onRoomDescriptionChange={vi.fn()}
        onRoomMemberIDsChange={vi.fn()}
        onClose={vi.fn()}
        onCreate={create}
        roomType="on_demand"
        managerAvailable={false}
        onRoomTypeChange={changeMode}
      />,
    );
    expect(screen.getByText("collaborationManagerRequired")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "create" })).toBeDisabled();
    await userEvent.setup().click(screen.getByRole("radio", { name: /freeCollaboration/ }));
    expect(changeMode).toHaveBeenCalledWith("free");
    expect(create).not.toHaveBeenCalled();
  });
});
