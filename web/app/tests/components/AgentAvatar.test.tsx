import { render } from "@testing-library/react";
import { AgentAvatarContent, AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";

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
    const { container } = render(
      <AgentAvatarPicker value="" t={(key) => key} onChange={() => {}} />,
    );

    expect(container.querySelectorAll(".agent-avatar-option.selected")).toHaveLength(0);
  });
});
