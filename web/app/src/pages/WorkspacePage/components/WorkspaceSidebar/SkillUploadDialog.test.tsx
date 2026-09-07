import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SkillUploadDialog } from "./SkillUploadDialog";

afterEach(() => vi.restoreAllMocks());

describe("SkillUploadDialog", () => {
  it.each([false, true])("opens the ZIP picker and uploads the selected file (remote mode: %s)", async (remote) => {
    const user = userEvent.setup();
    const onSubmit = vi.fn().mockResolvedValue(true);
    const onOpenChange = vi.fn();
    render(
      <SkillUploadDialog
        open
        busy={false}
        error=""
        installedSkills={[]}
        onOpenChange={onOpenChange}
        onSubmit={onSubmit}
        remoteInstallBusy=""
        remoteInstallError=""
        remoteSkills={[]}
        remoteSkillsError=""
        remoteSkillsHasMore={false}
        remoteSkillsLoading={false}
        remoteSkillsLoadingMore={false}
        remoteSkillsSearch=""
        t={(key) => key}
      />,
    );
    if (remote) {
      await user.click(screen.getByRole("tab", { name: "resourcesSkillRemoteInstallTab" }));
    }
    const openPicker = vi.spyOn(HTMLInputElement.prototype, "click");
    await user.click(screen.getByRole("tab", { name: "resourcesSkillUploadZipTab" }));
    expect(openPicker).toHaveBeenCalledOnce();
    const input = document.querySelector<HTMLInputElement>('input[type="file"]');
    expect(input).not.toBeNull();
    const file = new File(["skill archive"], "example.zip", { type: "application/zip" });
    await user.upload(input!, file);
    expect(screen.getByText("example.zip")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "resourcesSkillUploadSubmit" }));
    expect(onSubmit).toHaveBeenCalledWith(file);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
