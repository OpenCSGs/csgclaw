import type { ReactElement } from "react";
import { createHashRouter, RouterProvider } from "react-router-dom";
import type { RouteObject } from "react-router-dom";
import { AgentPage } from "@/pages/AgentPage";
import { ComputerPage } from "@/pages/ComputerPage";
import { ConversationPage } from "@/pages/ConversationPage";
import { HubPage } from "@/pages/HubPage";
import { WorkspacePage } from "@/pages/WorkspacePage/WorkspacePage";

const routes: RouteObject[] = [
  {
    path: "/",
    element: <WorkspacePage />,
    children: [
      { index: true, element: <ConversationPage /> },
      { path: "computer", element: <ComputerPage /> },
      { path: "agents/:agentId", element: <AgentPage /> },
      { path: "agent/:agentId", element: <AgentPage /> },
      { path: "hub", element: <HubPage /> },
      { path: "rooms/:conversationId", element: <ConversationPage /> },
      { path: "room/:conversationId", element: <ConversationPage /> },
      { path: "channels/:conversationId", element: <ConversationPage /> },
      { path: "channel/:conversationId", element: <ConversationPage /> },
      { path: "dms/:conversationId", element: <ConversationPage /> },
      { path: "dm/:conversationId", element: <ConversationPage /> },
      { path: "conversations/:conversationId", element: <ConversationPage /> },
      { path: "conversation/:conversationId", element: <ConversationPage /> },
      { path: "*", element: <ConversationPage /> },
    ],
  },
];

const router = createHashRouter(routes);

export function AppRouter(): ReactElement {
  return <RouterProvider router={router} />;
}
