import { fireEvent, render, screen } from "@testing-library/react";
import { HumanDetailPane } from "@/pages/HumanPage/components";
import type { IMUser, TranslateFn } from "@/models/conversations";

const labels: Record<string, string> = {
  handleLabel: "Handle",
  humanDetailTitle: "Human profile",
  humanIdentitySection: "Identity",
  humanStatusOffline: "Offline",
  humanStatusOnline: "Online",
  agentAvatar: "Avatar",
  agentAvatarStyle3D: "3D",
  agentAvatarStyleCartoon: "Cartoon",
  agentAvatarStylePic: "Portrait",
  avatarBuiltinTab: "Built-in avatars",
  avatarUploadTab: "Upload avatar",
  cancel: "Cancel",
  close: "Close",
  confirm: "Confirm",
  editAvatar: "Edit avatar",
  editAvatarSubtitle: "Choose or upload an avatar.",
  localComputer: "Local computer",
  profileLocalProvider: "Local",
  profileLocalRuntime: "Workspace",
  roleLabel: "Role",
  userIDLabel: "User ID",
};

const t: TranslateFn = (key) => labels[key] ?? key;

const admin: IMUser = {
  id: "u-admin",
  name: "Admin User",
  handle: "admin",
  role: "admin",
  avatar: "avatar/cartoon-1.png",
  is_online: true,
};

describe("HumanDetailPane", () => {
  it("renders human identity without the direct messages panel", () => {
    const { container } = render(<HumanDetailPane locale="en" t={t} user={admin} />);

    expect(screen.getByRole("heading", { name: "Admin User" })).toBeInTheDocument();
    expect(screen.getAllByText("@admin")).toHaveLength(1);
    expect(screen.getAllByText("User ID")).toHaveLength(1);
    expect(screen.getAllByText("u-admin")).toHaveLength(1);
    expect(screen.queryByText("Direct messages")).not.toBeInTheDocument();

    expect(screen.getByRole("button", { name: "Edit avatar: Cartoon 1" })).toBeInTheDocument();

    const sections = container.querySelectorAll(".human-info-section");
    const identityFields = container.querySelector(".human-identity-fields");
    const avatarSection = container.querySelector(".human-avatar-section");
    expect(sections).toHaveLength(1);
    expect(sections[0]).toHaveClass("human-identity-section");
    expect(identityFields).toBeInTheDocument();
    expect(identityFields?.children).toHaveLength(3);
    expect(avatarSection).not.toBeInTheDocument();
  });

  it("shows an empty state when the human is unavailable", () => {
    render(<HumanDetailPane t={t} user={null} />);

    expect(screen.getByText("humanDetailMissing")).toBeInTheDocument();
  });

  it("allows configuring the human avatar with the shared avatar picker", () => {
    const onAvatarChange = vi.fn();

    render(<HumanDetailPane locale="en" onAvatarChange={onAvatarChange} t={t} user={admin} />);

    fireEvent.click(screen.getByRole("button", { name: "Edit avatar: Cartoon 1" }));

    expect(screen.getByRole("dialog", { name: "Edit avatar" })).toBeInTheDocument();
    expect(screen.getByRole("radiogroup", { name: "Built-in avatars" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("radio", { name: "Cartoon 2" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    expect(onAvatarChange).toHaveBeenCalledWith("avatar/cartoon-2.png");
  });
});
