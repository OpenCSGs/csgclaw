import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DeleteConversationModal } from "@/pages/WorkspacePage/components/WorkspaceModals";

function t(key: string, params: Record<string, string | number> = {}) {
  const labels: Record<string, string> = {
    cancel: "Cancel",
    close: "Close",
    confirmDelete: "Delete",
    deleteDirectMessageConfirmDescription:
      'Are you sure you want to delete the DM with "{name}"? The chat history cannot be recovered.',
    deleteDirectMessageConfirmTitle: "Delete DM?",
    deleteRoomConfirmDescription:
      'Are you sure you want to delete "{name}"? The chat history and room content cannot be recovered.',
    deleteRoomConfirmTitle: "Delete room?",
    deleting: "Deleting...",
  };
  return (labels[key] || key).replace(/\{(\w+)\}/g, (_, name) => `${params[name] ?? ""}`);
}

describe("DeleteConversationModal", () => {
  it("warns before deleting a private conversation", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    const onConfirm = vi.fn();

    render(
      <DeleteConversationModal conversationTitle="dev2" isDirect onCancel={onCancel} onConfirm={onConfirm} t={t} />,
    );

    expect(screen.getByRole("alertdialog", { name: "Delete DM?" })).toBeInTheDocument();
    expect(
      screen.getByText('Are you sure you want to delete the DM with "dev2"?', { exact: false }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("warns before deleting a room", () => {
    render(
      <DeleteConversationModal
        conversationTitle="研发群"
        isDirect={false}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
        t={t}
      />,
    );

    expect(screen.getByRole("alertdialog", { name: "Delete room?" })).toBeInTheDocument();
    expect(screen.getByText('Are you sure you want to delete "研发群"?', { exact: false })).toBeInTheDocument();
  });
});
