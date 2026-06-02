import { render } from "@testing-library/react";
import { AgentAvatarContent, avatarFallbackText } from "@/components/business/AgentAvatar";

describe("AgentAvatar", () => {
  it("renders a fallback label when the avatar path is missing", () => {
    const { container } = render(<AgentAvatarContent avatar="" fallback="AB" />);

    expect(container.querySelector(".agent-avatar-image")).not.toBeInTheDocument();
    expect(container.querySelector(".agent-avatar-fallback")).toHaveTextContent("AB");
  });

  it("derives a short fallback label from the available identity fields", () => {
    expect(avatarFallbackText("", "Alice Bob", "alice", "u-alice")).toBe("AL");
    expect(avatarFallbackText("LU", "", "", "")).toBe("LU");
    expect(avatarFallbackText("", "", "", "")).toBe("#");
  });
});
