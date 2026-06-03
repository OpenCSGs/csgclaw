import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ManagerRebuildModal } from "@/pages/WorkspacePage/components";

const labels: Record<string, string> = {
  agentImage: "Image",
  agentImagePlaceholder: "Uses the manager image by default",
  close: "Close",
  managerRebuildAction: "Recreate",
  managerRebuildBusy: "Recreating...",
  managerRebuildSubtitle: "Choose runtime and image.",
  managerRebuildTitle: "Recreate Manager",
  profileRuntimeKind: "Runtime",
  runtimeOpenclaw: "OpenClaw",
  runtimePicoclaw: "PicoClaw",
};

function t(key: string): string {
  return labels[key] ?? key;
}

describe("ManagerRebuildModal", () => {
  it("lets users choose manager runtime and image before rebuilding", async () => {
    const user = userEvent.setup();
    const onRuntimeKindChange = vi.fn();
    const onImageChange = vi.fn();
    const onClose = vi.fn();
    const onConfirm = vi.fn();
    const managerImage = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.5.27";
    const localImage = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.5.22";
    const alternateImage = "picoclaw:alternate";

    const { container } = render(
      <ManagerRebuildModal
        t={t}
        runtimeOptions={[{ value: "picoclaw_sandbox" }, { value: "openclaw_sandbox" }]}
        runtimeKind="picoclaw_sandbox"
        image={managerImage}
        imageOptions={[managerImage, localImage, alternateImage]}
        templateVariants={[
          { runtimeKind: "picoclaw_sandbox", image: managerImage },
          { runtimeKind: "openclaw_sandbox", image: "openclaw:template" },
        ]}
        bootstrapConfig={{
          runtime_default_images: {
            openclaw_sandbox: "openclaw:manager",
            picoclaw_sandbox: managerImage,
          },
        }}
        managerAgent={{ image: "fallback:manager" }}
        busy={false}
        error=""
        onRuntimeKindChange={onRuntimeKindChange}
        onImageChange={onImageChange}
        onClose={onClose}
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText("Recreate Manager")).toBeInTheDocument();
    const runtimeSelect = screen.getByRole("combobox", { name: "Runtime" });

    expect(runtimeSelect).toHaveTextContent("PicoClaw");
    const imageSelect = screen.getByRole("combobox", { name: "Image" });
    expect(imageSelect).toHaveAttribute("title", managerImage);
    const selectedImageLabel = imageSelect.querySelector(".manager-rebuild-image-option");
    expect(selectedImageLabel).toBeInTheDocument();
    expect(selectedImageLabel?.querySelector(".manager-rebuild-image-name")).toHaveTextContent("picoclaw");
    expect(selectedImageLabel?.querySelector(".manager-rebuild-image-tag")).toHaveTextContent(":2026.5.27");
    expect(selectedImageLabel?.querySelector(".manager-rebuild-image-context")).toHaveTextContent(
      "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq",
    );

    await user.click(imageSelect);
    expect(await screen.findByRole("option", { name: managerImage })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: localImage })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: alternateImage })).toBeInTheDocument();
    const managerOption = screen.getByRole("option", { name: managerImage });
    expect(managerOption).toHaveClass("manager-rebuild-image-select-item");
    expect(managerOption.querySelector(".manager-rebuild-image-name")).toHaveTextContent("picoclaw");
    expect(managerOption.querySelector(".manager-rebuild-image-context")).toHaveTextContent(
      "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq",
    );
    await user.click(screen.getByRole("option", { name: alternateImage }));
    expect(onImageChange).toHaveBeenCalledWith(alternateImage);

    await user.click(runtimeSelect);
    await user.click(await screen.findByRole("option", { name: "OpenClaw" }));
    expect(onRuntimeKindChange).toHaveBeenCalledWith("openclaw_sandbox");
    expect(onImageChange).toHaveBeenCalledWith("openclaw:manager");

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
