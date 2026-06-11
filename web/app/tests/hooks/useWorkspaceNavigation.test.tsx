import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { BrowserRouter, useLocation, useNavigate } from "react-router-dom";
import { useWorkspaceNavigation } from "@/hooks/workspace/useWorkspaceNavigation";
import { paneFromLocation } from "@/models/routing";
import type { IMConversation } from "@/models/conversations";

const rooms: IMConversation[] = [
  {
    id: "room-1",
    is_direct: false,
    members: [],
    messages: [],
    title: "Room 1",
  },
];

function NavigationHarness() {
  const location = useLocation();
  const navigate = useNavigate();
  const [activeConversationId, setActiveConversationId] = useState("room-1");

  const activePane = paneFromLocation(location.pathname);
  const navigation = useWorkspaceNavigation({
    dataReady: true,
    location,
    navigate,
    rooms,
    setActiveConversationId,
  });

  return (
    <>
      <div data-testid="path">{location.pathname}</div>
      <div data-testid="pane">
        {activePane.type}:{activePane.id}
      </div>
      <div data-testid="conversation">{activeConversationId}</div>
      <button type="button" onClick={() => navigation.selectAgent({ id: "agent-1" })}>
        Open agent
      </button>
      <button type="button" onClick={() => navigation.selectTeam({ id: "team-1" })}>
        Open team
      </button>
      <button type="button" onClick={() => navigation.selectHuman({ id: "u-admin" })}>
        Open human
      </button>
    </>
  );
}

describe("useWorkspaceNavigation", () => {
  afterEach(() => {
    window.history.replaceState({}, "", "/");
  });

  it("lets browser navigation update the active pane before normalizing the URL", async () => {
    window.history.replaceState({}, "", "/agents/u-manager");

    render(
      <BrowserRouter>
        <NavigationHarness />
      </BrowserRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("pane")).toHaveTextContent("agent:u-manager");
    });
    expect(screen.getByTestId("path")).toHaveTextContent("/agents/u-manager");
  });

  it("normalizes route aliases through the router without pane state", async () => {
    window.history.replaceState({}, "", "/agent/u-manager");

    render(
      <BrowserRouter>
        <NavigationHarness />
      </BrowserRouter>,
    );

    expect(screen.getByTestId("pane")).toHaveTextContent("agent:u-manager");
    await waitFor(() => {
      expect(screen.getByTestId("path")).toHaveTextContent("/agents/u-manager");
    });
  });

  it("selects panes by navigating and deriving active pane from the URL", async () => {
    window.history.replaceState({}, "", "/rooms/room-1");

    render(
      <BrowserRouter>
        <NavigationHarness />
      </BrowserRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open agent" }));

    await waitFor(() => {
      expect(screen.getByTestId("path")).toHaveTextContent("/agents/agent-1");
    });
    expect(screen.getByTestId("pane")).toHaveTextContent("agent:agent-1");
  });

  it("selects teams without opening the backing room", async () => {
    window.history.replaceState({}, "", "/rooms/room-1");

    render(
      <BrowserRouter>
        <NavigationHarness />
      </BrowserRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open team" }));

    await waitFor(() => {
      expect(screen.getByTestId("path")).toHaveTextContent("/teams/team-1");
    });
    expect(screen.getByTestId("pane")).toHaveTextContent("team:team-1");
  });

  it("selects humans by opening the human detail route", async () => {
    window.history.replaceState({}, "", "/rooms/room-1");

    render(
      <BrowserRouter>
        <NavigationHarness />
      </BrowserRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open human" }));

    await waitFor(() => {
      expect(screen.getByTestId("path")).toHaveTextContent("/humans/u-admin");
    });
    expect(screen.getByTestId("pane")).toHaveTextContent("human:u-admin");
  });
});
