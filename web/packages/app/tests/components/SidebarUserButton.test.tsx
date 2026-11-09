import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SidebarUserButton } from "@/pages/WorkspacePage/components/WorkspaceSidebar/SidebarUserButton";

it.each(["icon", "row"] as const)("opens settings from the %s button", async (presentation) => {
  const onOpenSettings = vi.fn();
  render(<SidebarUserButton presentation={presentation} onOpenSettings={onOpenSettings} t={(key) => key} />);
  await userEvent.setup().click(screen.getByRole("button", { name: "settings" }));
  expect(onOpenSettings).toHaveBeenCalledOnce();
  expect(screen.queryByRole("menu")).not.toBeInTheDocument();
});

it("indicates the active settings page", () => {
  render(<SidebarUserButton active onOpenSettings={vi.fn()} t={(key) => key} />);
  expect(screen.getByRole("button", { name: "settings" })).toHaveAttribute("aria-current", "page");
});
