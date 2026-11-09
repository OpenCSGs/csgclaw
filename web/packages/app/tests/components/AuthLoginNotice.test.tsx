import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuthLoginNotice } from "@/pages/WorkspacePage/components/AuthLoginNotice/AuthLoginNotice";
import type { AuthNotice } from "@/hooks/workspace/useAuthController";

const notice: AuthNotice = {
  id: "login-success",
  avatarFallback: "AL",
  title: "Signed in",
  message: "User alice signed in.",
  type: "login",
  tone: "success",
};

describe("AuthLoginNotice", () => {
  it("renders the login result with the Radix toast", () => {
    render(<AuthLoginNotice closeLabel="Close" notice={notice} />);

    expect(screen.getByText("Signed in")).toBeInTheDocument();
    expect(screen.getByText("User alice signed in.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();
  });

  it("uses the Radix release that stabilizes DismissableLayer refs under React 19", () => {
    const require = createRequire(import.meta.url);
    const packageJSON = JSON.parse(readFileSync(require.resolve("radix-ui/package.json"), "utf8")) as {
      dependencies: Record<string, string>;
    };
    const version = packageJSON.dependencies["@radix-ui/react-dismissable-layer"];
    const [major, minor, patch] = version.split(".").map(Number);

    expect(major > 1 || (major === 1 && (minor > 1 || (minor === 1 && patch >= 14)))).toBe(true);
  });

  it("dismisses once when the close button is clicked", async () => {
    const user = userEvent.setup();
    const onDismiss = vi.fn();

    render(<AuthLoginNotice closeLabel="Close" notice={notice} onDismiss={onDismiss} />);

    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
