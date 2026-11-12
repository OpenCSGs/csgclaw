import { StrictMode } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuthLoginNotice } from "./AuthLoginNotice";

describe("AuthLoginNotice", () => {
  it("renders and dismisses the completed login notice without a ref update loop", async () => {
    const user = userEvent.setup();
    const onDismiss = vi.fn();

    render(
      <StrictMode>
        <AuthLoginNotice
          closeLabel="Close"
          notice={{
            id: "auth-login-complete-1",
            avatarFallback: "A",
            title: "Signed in",
            message: "alice signed in.",
            type: "login",
            tone: "success",
          }}
          onDismiss={onDismiss}
        />
      </StrictMode>,
    );

    expect(screen.getByText("Signed in")).toBeInTheDocument();
    expect(screen.getByText("alice signed in.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
