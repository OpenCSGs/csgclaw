import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SkillSummary } from "@/models/skillhub";
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
    expect(openPicker).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: /resourcesSkillUploadDropTitle/ }));
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

  it("disables the upload button when no file is selected in zip mode", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn().mockResolvedValue(true);
    render(
      <SkillUploadDialog
        open
        busy={false}
        error=""
        installedSkills={[]}
        onOpenChange={vi.fn()}
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

    // No file selected yet — button should be disabled
    const uploadButton = screen.getByRole("button", { name: "resourcesSkillUploadSubmit" });
    expect(uploadButton).toBeDisabled();

    // Clicking a disabled button should not trigger submit
    await user.click(uploadButton);
    expect(onSubmit).not.toHaveBeenCalled();

    // Now select a file — button should become enabled
    const input = document.querySelector<HTMLInputElement>('input[type="file"]');
    expect(input).not.toBeNull();
    const file = new File(["skill archive"], "example.zip", { type: "application/zip" });
    await user.upload(input!, file);
    expect(uploadButton).not.toBeDisabled();
  });

  it("requests remote access again when the selected remote tab is clicked again", async () => {
    const user = userEvent.setup();
    const onRemoteVisibleChange = vi.fn();
    render(
      <SkillUploadDialog
        open
        busy={false}
        error=""
        installedSkills={[]}
        onOpenChange={() => undefined}
        onRemoteVisibleChange={onRemoteVisibleChange}
        onSubmit={vi.fn()}
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

    const remoteTab = screen.getByRole("tab", { name: "resourcesSkillRemoteInstallTab" });
    await user.click(remoteTab);
    await user.click(remoteTab);

    expect(onRemoteVisibleChange.mock.calls.filter(([visible]) => visible)).toHaveLength(2);
  });
});

describe("远端 skill 安装操作", () => {
  it.each<{ label: string; installedSkills: SkillSummary[]; action: string | null }>([
    {
      label: "system 来源",
      installedSkills: [{ name: "agent-builder", source: "system", readonly: true }],
      action: null,
    },
    { label: "builtin 来源", installedSkills: [{ name: "agent-builder", source: "builtin" }], action: null },
    { label: "只读内置 skill", installedSkills: [{ name: "agent-builder", readonly: true }], action: null },
    {
      label: "同名本地 skill",
      installedSkills: [{ name: "agent-builder", source: "local" }],
      action: "resourcesSkillRemoteReplaceAction",
    },
    {
      label: "其他名称的内置 skill",
      installedSkills: [{ name: "another-skill", source: "system" }],
      action: "resourcesSkillRemoteInstallAction",
    },
  ])("$label", async ({ installedSkills, action }) => {
    const user = userEvent.setup();
    const onInstallRemoteSkill = vi.fn().mockResolvedValue(true);
    const remoteSkill = {
      name: "Agent Builder",
      remotePath: "AIWizards/agent-builder",
      source: "official",
      readonly: true,
    };
    render(
      <SkillUploadDialog
        open
        busy={false}
        error=""
        installedSkills={installedSkills}
        onOpenChange={vi.fn()}
        onSubmit={vi.fn()}
        onInstallRemoteSkill={onInstallRemoteSkill}
        remoteInstallBusy=""
        remoteInstallError=""
        remoteSkills={[remoteSkill]}
        remoteSkillsError=""
        remoteSkillsHasMore={false}
        remoteSkillsLoading={false}
        remoteSkillsLoadingMore={false}
        remoteSkillsSearch=""
        t={(key) => key}
      />,
    );
    await user.click(screen.getByRole("tab", { name: "resourcesSkillRemoteInstallTab" }));
    expect(screen.getByText("Agent Builder")).toBeVisible();
    if (action) {
      await user.click(screen.getByRole("button", { name: action }));
      expect(onInstallRemoteSkill).toHaveBeenCalledWith(remoteSkill, {
        replace: action === "resourcesSkillRemoteReplaceAction",
      });
    } else {
      expect(screen.queryByRole("button", { name: "resourcesSkillRemoteReplaceAction" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "resourcesSkillRemoteInstallAction" })).not.toBeInTheDocument();
      expect(onInstallRemoteSkill).not.toHaveBeenCalled();
    }
  });
});
