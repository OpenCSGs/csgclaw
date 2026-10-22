import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { HumanPage } from "./HumanPage";

const { onAvatarChange, close } = vi.hoisted(() => ({ onAvatarChange: vi.fn(), close: vi.fn() }));
vi.mock("@/hooks/workspace", () => ({
  useWorkspaceControllerContext: () => ({
    ready: true,
    t: (key: string) => key,
    humanViewProps: {
      user: { id: "user-admin", name: "Local user", role: "admin" },
      t: (key: string) => key,
      onAvatarChange,
    },
  }),
}));
vi.mock("@/hooks/workspace/useWorkspaceDialogs", () => ({
  useWorkspaceDialogs: () => ({ current: "profile", close }),
}));
afterEach(() => vi.clearAllMocks());

it("edits the personal avatar inside the profile dialog without closing the profile", async () => {
  const user = userEvent.setup();
  render(<HumanPage />);
  const profile = screen.getByRole("dialog", { name: "humanSection" });
  await user.click(within(profile).getByRole("button", { name: "editAvatar" }));
  const editor = within(profile).getByRole("dialog", { name: "editAvatar" });
  await user.click(within(editor).getAllByRole("radio")[0]);
  await user.click(within(editor).getByRole("button", { name: "confirm" }));
  expect(onAvatarChange).toHaveBeenCalledOnce();
  expect(onAvatarChange).toHaveBeenCalledWith(expect.stringMatching(/^avatar\//));
  expect(screen.queryByRole("dialog", { name: "editAvatar" })).not.toBeInTheDocument();
  expect(profile).toBeVisible();
  expect(close).not.toHaveBeenCalled();
});

it("keeps the profile open when Escape closes the avatar editor", async () => {
  const user = userEvent.setup();
  render(<HumanPage />);
  await user.click(screen.getByRole("button", { name: "editAvatar" }));
  const editor = screen.getByRole("dialog", { name: "editAvatar" });
  await user.click(within(editor).getByRole("tab", { name: "avatarUploadTab" }));
  expect(within(editor).getByRole("tab", { name: "avatarUploadTab" })).toHaveAttribute("aria-selected", "true");
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "editAvatar" })).not.toBeInTheDocument();
  expect(screen.getByRole("dialog", { name: "humanSection" })).toBeVisible();
  expect(close).not.toHaveBeenCalled();
});
