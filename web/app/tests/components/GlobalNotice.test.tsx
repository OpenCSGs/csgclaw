import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GlobalNoticeProvider, useGlobalNotice } from "@/components/ui";

function Harness() {
  const { showNotice } = useGlobalNotice();
  return (
    <button
      type="button"
      onClick={() =>
        showNotice({
          title: "Install Node.js first",
          message: "Install Node.js 24 LTS and try again.",
          closeLabel: "Close",
          tone: "warning",
        })
      }
    >
      Show notice
    </button>
  );
}

describe("GlobalNotice", () => {
  it("shows and dismisses an app-level warning", async () => {
    const user = userEvent.setup();
    render(
      <GlobalNoticeProvider>
        <Harness />
      </GlobalNoticeProvider>,
    );

    await user.click(screen.getByRole("button", { name: "Show notice" }));
    expect(screen.getByText("Install Node.js first")).toBeInTheDocument();
    expect(screen.getByText("Install Node.js 24 LTS and try again.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByText("Install Node.js first")).not.toBeInTheDocument();
  });
});
