import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ManagerRebuildModal } from "@/pages/WorkspacePage/components";

const labels: Record<string, string> = {
  agentImage: "Image",
  agentImagePlaceholder: "Uses the manager image by default",
  close: "Close",
  managerRebuildAction: "Recreate",
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
    const onConfirm = vi.fn();

    render(
      <ManagerRebuildModal
        t={t}
        runtimeOptions={[{ value: "picoclaw_sandbox" }, { value: "openclaw_sandbox" }]}
        runtimeKind="picoclaw_sandbox"
        image="picoclaw:manager"
        imageOptions={["picoclaw:manager", "picoclaw:alternate"]}
        templateVariants={[
          { runtimeKind: "picoclaw_sandbox", image: "picoclaw:manager" },
          { runtimeKind: "openclaw_sandbox", image: "openclaw:template" },
        ]}
        bootstrapConfig={{
          runtime_default_images: {
            openclaw_sandbox: "openclaw:manager",
            picoclaw_sandbox: "picoclaw:manager",
          },
        }}
        managerAgent={{ image: "fallback:manager" }}
        busy={false}
        error=""
        onRuntimeKindChange={onRuntimeKindChange}
        onImageChange={onImageChange}
        onClose={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText("Recreate Manager")).toBeInTheDocument();
    const runtimeSelect = screen.getByRole("combobox", { name: "Runtime" });

    expect(runtimeSelect).toHaveTextContent("PicoClaw");
    expect(screen.getByLabelText("Image")).toHaveValue("picoclaw:manager");
    expect(document.querySelector('option[value="picoclaw:alternate"]')).toBeInTheDocument();

    await user.click(runtimeSelect);
    await user.click(await screen.findByRole("option", { name: "OpenClaw" }));
    expect(onRuntimeKindChange).toHaveBeenCalledWith("openclaw_sandbox");
    expect(onImageChange).toHaveBeenCalledWith("openclaw:template");

    await user.click(screen.getByRole("button", { name: "Recreate" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
