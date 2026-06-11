import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ManagerRebuildModal } from "@/pages/WorkspacePage/components";

const labels: Record<string, string> = {
  agentImage: "Image",
  agentImagePlaceholder: "Uses the manager image by default",
  close: "Close",
  managerRebuildAction: "Recreate",
  managerRebuildBusy: "Recreating...",
  managerRebuildSubtitle: "Choose runtime.",
  managerRebuildTitle: "Recreate Manager",
  profileRuntimeKind: "Runtime",
  runtimeOpenclaw: "OpenClaw",
  runtimePicoclaw: "PicoClaw",
};

function t(key: string): string {
  return labels[key] ?? key;
}

describe("ManagerRebuildModal", () => {
  it("lets users choose manager runtime and shows the resolved image without image selection", async () => {
    const user = userEvent.setup();
    const onRuntimeKindChange = vi.fn();
    const onClose = vi.fn();
    const onConfirm = vi.fn();
    const managerImage = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.6.8";

    const { container } = render(
      <ManagerRebuildModal
        t={t}
        runtimeOptions={[{ value: "picoclaw_sandbox" }, { value: "openclaw_sandbox" }]}
        runtimeKind="picoclaw_sandbox"
        image={managerImage}
        busy={false}
        error=""
        onRuntimeKindChange={onRuntimeKindChange}
        onClose={onClose}
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText("Recreate Manager")).toBeInTheDocument();
    const runtimeSelect = screen.getByRole("combobox", { name: "Runtime" });

    expect(runtimeSelect).toHaveTextContent("PicoClaw");
    expect(screen.queryByRole("combobox", { name: "Image" })).not.toBeInTheDocument();
    const imageDisplay = container.querySelector(".manager-rebuild-image-readonly");
    expect(imageDisplay).toHaveAttribute("title", managerImage);
    const selectedImageLabel = imageDisplay?.querySelector(".manager-rebuild-image-option");
    expect(selectedImageLabel).toBeInTheDocument();
    expect(selectedImageLabel?.querySelector(".manager-rebuild-image-name")).toHaveTextContent("picoclaw");
    expect(selectedImageLabel?.querySelector(".manager-rebuild-image-tag")).toHaveTextContent(":2026.6.8");
    expect(selectedImageLabel?.querySelector(".manager-rebuild-image-context")).toHaveTextContent(
      "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq",
    );
    expect(screen.queryByRole("option", { name: managerImage })).not.toBeInTheDocument();

    await user.click(runtimeSelect);
    await user.click(await screen.findByRole("option", { name: "OpenClaw" }));
    expect(onRuntimeKindChange).toHaveBeenCalledWith("openclaw_sandbox");

    await user.click(screen.getByRole("button", { name: "Recreate" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);

    const backdrop = container.querySelector(".modal-backdrop");
    expect(backdrop).toBeInstanceOf(HTMLElement);
    await user.click(backdrop as HTMLElement);
    expect(onClose).not.toHaveBeenCalled();

    await user.click(screen.getAllByRole("button", { name: "Close" })[0]);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
