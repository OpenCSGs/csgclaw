import { createRef } from "react";
import { render, screen, within } from "@testing-library/react";
import { ProfilePreviewPopover } from "@/pages/WorkspacePage/components/ProfilePreviewPopover";

const labels: Record<string, string> = {
  agentDelete: "Delete",
  agentStatusUnknown: "Unknown",
  close: "Close",
  offline: "Offline",
  online: "Online",
  openDM: "DM",
  openProfile: "Open",
  profileCompleteBadge: "Complete",
  profileLocalProvider: "CSGClaw",
  profileLocalRuntime: "Local",
  profileModel: "Model",
  profilePreview: "Profile preview",
  profileProvider: "Provider",
  profileRestartRequired: "Restart required",
  profileRuntimeKind: "Runtime",
  roleLabel: "Role",
  runtimeOpenclaw: "OpenClaw",
  runtimePicoclaw: "PicoClaw",
  personProfile: "Person profile",
  status: "Status",
  userIDLabel: "User ID",
  "roles.admin": "admin",
  "roles.worker": "Worker",
};

function t(key: string): string {
  return labels[key] ?? key;
}

describe("ProfilePreviewPopover", () => {
  it("shows compact agent runtime/provider/model fields with reasoning in model", () => {
    render(
      <ProfilePreviewPopover
        previewRef={createRef<HTMLElement>()}
        agent={{
          id: "agent-1",
          name: "Builder",
          role: "worker",
          status: "running",
          provider: "api",
          model_id: "gpt-5.5",
          reasoning_effort: "medium",
          runtime_kind: "codex",
        }}
        user={{ id: "u-builder" }}
        anchorRect={{ top: 20, right: 80, bottom: 60, left: 40 }}
        t={t}
        onClose={vi.fn()}
        onOpenAgent={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    const fields = screen.getByText("STATUS").closest(".preview-fields");
    expect(fields).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("STATUS")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("RUNTIME")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("PROVIDER")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("MODEL")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("Codex")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("OpenAI API")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("gpt-5.5(medium)")).toBeInTheDocument();
    expect(within(fields as HTMLElement).queryByText("Reasoning")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "DM" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("shows DM for manager agent previews", () => {
    render(
      <ProfilePreviewPopover
        previewRef={createRef<HTMLElement>()}
        agent={{
          id: "u-manager",
          name: "manager",
          role: "manager",
          status: "running",
          provider: "api",
          model_id: "gpt-5.5",
          runtime_kind: "picoclaw_sandbox",
        }}
        user={{ id: "u-manager" }}
        anchorRect={{ top: 20, right: 80, bottom: 60, left: 40 }}
        t={t}
        onClose={vi.fn()}
        onOpenAgent={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Open" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "DM" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("uses agent-style metadata fields for local admin users", () => {
    render(
      <ProfilePreviewPopover
        previewRef={createRef<HTMLElement>()}
        agent={null}
        user={{
          accent_hex: "#dc2626",
          avatar: "LU",
          id: "u-admin",
          name: "Local user",
          role: "admin",
        }}
        anchorRect={{ top: 20, right: 80, bottom: 60, left: 40 }}
        t={t}
        onClose={vi.fn()}
        onOpenAgent={vi.fn()}
        onOpenDM={vi.fn()}
      />,
    );

    expect(screen.getByText("Profile preview")).toBeInTheDocument();
    expect(screen.queryByText("Person profile")).not.toBeInTheDocument();

    const fields = screen.getByText("STATUS").closest(".preview-fields");
    expect(fields).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("STATUS")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("RUNTIME")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("PROVIDER")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("MODEL")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("Local")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("CSGClaw")).toBeInTheDocument();
    expect(within(fields as HTMLElement).getByText("admin")).toBeInTheDocument();
    expect(within(fields as HTMLElement).queryByText("ROLE")).not.toBeInTheDocument();
    expect(within(fields as HTMLElement).queryByText("HANDLE")).not.toBeInTheDocument();
    expect(within(fields as HTMLElement).queryByText("USER ID")).not.toBeInTheDocument();
  });
});
