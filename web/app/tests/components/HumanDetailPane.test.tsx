import { fireEvent, render, screen } from "@testing-library/react";
import { HumanDetailPane } from "@/pages/HumanPage/components";
import type { IMUser, TranslateFn } from "@/models/conversations";

const labels: Record<string, string> = {
  humanDetailTitle: "Human profile",
  humanIdentitySection: "Identity",
  humanChannelsSection: "Channels",
  humanDescriptionLabel: "Description",
  humanStatusOffline: "Offline",
  humanStatusOnline: "Online",
  agentSavingChanges: "Saving...",
  agentSaved: "Saved",
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
  feishuChannelName: "Feishu",
  feishuConnected: "Connected",
  feishuDisconnected: "Disconnected",
};

const t: TranslateFn = (key) => labels[key] ?? key;

const admin: IMUser = {
  id: "u-admin",
  name: "Admin User",
  role: "admin",
  avatar: "avatar/cartoon-1.png",
  is_online: true,
  description: "Agents can @admin to double-check risky changes.",
  participants: [
    {
      channel: "feishu",
      channel_user_kind: "open_id",
      channel_user_ref: "ou_admin",
      id: "admin",
      name: "龙韵",
      type: "human",
    },
  ],
};

describe("HumanDetailPane", () => {
  it("renders human identity without the direct messages panel", () => {
    const { container } = render(<HumanDetailPane locale="en" t={t} user={admin} />);

    expect(screen.getByRole("heading", { name: "Admin User" })).toBeInTheDocument();
    expect(screen.getAllByText("User ID")).toHaveLength(1);
    expect(screen.getAllByText("u-admin")).toHaveLength(1);
    expect(screen.queryByText("Direct messages")).not.toBeInTheDocument();

    expect(screen.getByRole("button", { name: "Edit avatar: Cartoon 1" })).toBeInTheDocument();

    const identityFields = container.querySelector(".human-identity-fields");
    const avatarSection = container.querySelector(".human-avatar-section");
    expect(container.querySelector(".human-identity-section")).toBeInTheDocument();
    expect(identityFields).toBeInTheDocument();
    expect(identityFields?.children).toHaveLength(2);
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

  it("shows Feishu channel status without bound user detail", () => {
    const { container } = render(<HumanDetailPane locale="en" t={t} user={admin} />);

    expect(screen.getByRole("heading", { name: "Channels" })).toBeInTheDocument();
    expect(screen.getByText("Feishu")).toBeInTheDocument();
    expect(screen.getByText("Connected")).toBeInTheDocument();
    expect(screen.queryByText("龙韵")).not.toBeInTheDocument();
    expect(screen.queryByText("ou_admin")).not.toBeInTheDocument();

    const panel = container.querySelector(".human-info-panel");
    expect(panel?.firstElementChild).toHaveClass("human-channels-section");
  });

  it("autosaves the human description on blur", () => {
    const onDescriptionSave = vi.fn();
    render(<HumanDetailPane locale="en" onDescriptionSave={onDescriptionSave} t={t} user={admin} />);

    expect(screen.getByText("Saved")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Description" }));
    const description = screen.getByLabelText("Description");
    fireEvent.change(description, {
      target: { value: "Ask me to confirm product decisions." },
    });
    fireEvent.blur(description);

    expect(onDescriptionSave).toHaveBeenCalledWith("Ask me to confirm product decisions.");
  });
});
