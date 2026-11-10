import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Link, Outlet, useLocation } from "react-router-dom";

vi.mock("@/pages/WorkspacePage/WorkspacePage", () => ({
  WorkspacePage: () => (
    <>
      <Link to="/scheduled-tasks">Scheduled</Link>
      <Link to="/tasks/task-1">Detail</Link>
      <Outlet />
    </>
  ),
}));
vi.mock("@/pages/StartTaskPage/StartTaskPage", () => ({ StartTaskPage: () => <h1>Home</h1> }));
vi.mock("@/pages/TasksPage", () => ({ TasksPage: () => <div data-testid="task-route">{useLocation().pathname}</div> }));

describe("task routes", () => {
  it("redirects the retired board home while preserving scheduled tasks and task detail routes", async () => {
    window.history.replaceState({}, "", "/#/tasks");
    const { AppRouter } = await import("@/routes/AppRouter");
    render(<AppRouter />);
    expect(await screen.findByRole("heading", { name: "Home" })).toBeInTheDocument();
    await waitFor(() => expect(window.location.hash).toBe("#/"));
    expect(screen.queryByTestId("task-route")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: "Scheduled" }));
    await waitFor(() => expect(screen.getByTestId("task-route")).toHaveTextContent("/scheduled-tasks"));
    fireEvent.click(screen.getByRole("link", { name: "Detail" }));
    await waitFor(() => expect(screen.getByTestId("task-route")).toHaveTextContent("/tasks/task-1"));
  });
});
