import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentAvatarContent, AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import { createTranslator } from "@/shared/i18n";

describe("AgentAvatar", () => {
  it("renders a fallback label when the avatar path is missing", () => {
    const { container } = render(<AgentAvatarContent avatar="" fallback="AB" />);

    expect(container.querySelector(".agent-avatar-image")).not.toBeInTheDocument();
    expect(container.querySelector(".agent-avatar-fallback")).toHaveTextContent("AB");
  });

  it("derives a short fallback label from the available identity fields", () => {
    expect(avatarFallbackText("", "Alice Bob", "alice", "u-alice")).toBe("A");
    expect(avatarFallbackText("LU", "", "", "")).toBe("L");
    expect(avatarFallbackText("", "测试头像", "", "")).toBe("测");
    expect(avatarFallbackText("", "", "", "")).toBe("#");
  });

  it("does not preselect an avatar when none is provided", () => {
    const { container } = render(<AgentAvatarPicker value="" t={(key) => key} onChange={() => {}} />);

    expect(container.querySelectorAll(".agent-avatar-option.selected")).toHaveLength(0);
  });

  it.each(["zh", "en"] as const)("applies the chosen avatar only after confirmation in %s", async (locale) => {
    const t = createTranslator(locale);
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<AgentAvatarPicker mode="edit" value="avatar/cartoon-1.png" t={t} onChange={onChange} />);
    const trigger = screen.getByRole("button", { name: new RegExp(t("editAvatar")) });
    await user.click(trigger);
    let editor = screen.getByRole("dialog", { name: t("editAvatar") });
    const option = `${t("agentAvatarStyleCartoon")} 2`;
    await user.click(within(editor).getByRole("radio", { name: option }));
    expect(onChange).not.toHaveBeenCalled();
    await user.click(within(editor).getByRole("button", { name: t("cancel") }));
    expect(onChange).not.toHaveBeenCalled();

    await user.click(trigger);
    editor = screen.getByRole("dialog", { name: t("editAvatar") });
    expect(within(editor).getByRole("radio", { name: `${t("agentAvatarStyleCartoon")} 1` })).toBeChecked();
    await user.click(within(editor).getByRole("tab", { name: t("avatarUploadTab") }));
    expect(within(editor).getByRole("button", { name: t("avatarApply") })).toBeDisabled();
    await user.click(within(editor).getByRole("tab", { name: t("avatarBuiltinTab") }));
    await user.click(within(editor).getByRole("radio", { name: option }));
    await user.click(within(editor).getByRole("button", { name: t("avatarApply") }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith("avatar/cartoon-2.png");
    expect(screen.queryByRole("dialog", { name: t("editAvatar") })).not.toBeInTheDocument();
  });
});
