import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ManagerRebuildModal } from "@/pages/WorkspacePage/components";

const labels: Record<string, string> = {
  agentImage: "Image",
  agentImagePlaceholder: "Uses the manager image by default",
  close: "Close",
  managerRebuildAction: "Recreate",
  managerRebuildBusy: "Recreating...",
  managerRebuildSubtitle: "Manager runs on Codex CLI.",
  managerRebuildTitle: "Recreate Manager",
  profileRuntimeKind: "Runtime",
  runtimeCodexCLI: "Codex CLI",
  runtimeOpenclaw: "OpenClaw",
  runtimePicoclaw: "PicoClaw",
};

function t(key: string): string {
  return labels[key] ?? key;
}

describe("ManagerRebuildModal", () => {
  it("shows a fixed Codex runtime with no runtime or image selection", async () => {
    const user = userEvent.setup();
    const onRuntimeKindChange = vi.fn();
    const onClose = vi.fn();
    const onConfirm = vi.fn();

    const { container } = render(
      <ManagerRebuildModal
        t={t}
        runtimeOptions={[{ value: "codex" }]}
        runtimeKind="codex"
        image=""
        busy={false}
        error=""
        onRuntimeKindChange={onRuntimeKindChange}
        onClose={onClose}
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText("Recreate Manager")).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Runtime" })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Image" })).not.toBeInTheDocument();
    expect(container.querySelector(".manager-rebuild-runtime-field .manager-rebuild-image-readonly")).toHaveTextContent(
      "Codex CLI",
    );
    expect(screen.getByText("Uses the manager image by default")).toBeInTheDocument();
    expect(screen.queryByRole("option")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Recreate" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onRuntimeKindChange).not.toHaveBeenCalled();

    const backdrop = container.querySelector(".modal-backdrop");
    expect(backdrop).toBeInstanceOf(HTMLElement);
    await user.click(backdrop as HTMLElement);
    expect(onClose).not.toHaveBeenCalled();

    await user.click(screen.getAllByRole("button", { name: "Close" })[0]);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("blocks rebuild while Codex CLI is unavailable", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();

    render(
      <ManagerRebuildModal
        t={t}
        runtimeOptions={[{ value: "codex" }]}
        runtimeKind="codex"
        image=""
        busy={false}
        error=""
        runtimeWarning="Install Codex CLI first."
        onRuntimeKindChange={vi.fn()}
        onClose={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText("Install Codex CLI first.")).toBeInTheDocument();
    const submit = screen.getByRole("button", { name: "Recreate" });
    expect(submit).toBeDisabled();

    await user.click(submit);
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
