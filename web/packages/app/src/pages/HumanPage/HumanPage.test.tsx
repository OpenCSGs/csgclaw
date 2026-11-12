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

it("edits the personal avatar above the profile dialog without closing the profile", async () => {
  const user = userEvent.setup();
  render(<HumanPage />);
  const profile = screen.getByRole("dialog", { name: "profileLabel" });
  await user.click(within(profile).getByRole("button", { name: "editAvatar" }));
  const editor = screen.getByRole("dialog", { name: "editAvatar" });
  expect(profile).not.toContainElement(editor);
  await user.click(within(editor).getAllByRole("radio")[0]);
  await user.click(within(editor).getByRole("button", { name: "avatarApply" }));
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
  expect(screen.getByRole("dialog", { name: "profileLabel" })).toBeVisible();
  expect(close).not.toHaveBeenCalled();
});

it("traps avatar editor focus and returns it to the profile when canceled", async () => {
  const user = userEvent.setup();
  render(<HumanPage />);
  const trigger = screen.getByRole("button", { name: "editAvatar" });
  await user.click(trigger);
  const editor = screen.getByRole("dialog", { name: "editAvatar" });
  within(editor).getByRole("button", { name: "avatarApply" }).focus();
  await user.tab();
  expect(editor).toContainElement(document.activeElement as HTMLElement);
  await user.click(within(editor).getByRole("button", { name: "cancel" }));
  expect(trigger).toHaveFocus();
  expect(onAvatarChange).not.toHaveBeenCalled();
  expect(close).not.toHaveBeenCalled();
});
