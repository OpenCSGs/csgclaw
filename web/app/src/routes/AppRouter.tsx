import { lazy, Suspense } from "react";
import type { ComponentType, ReactElement } from "react";
import { createHashRouter, RouterProvider } from "react-router-dom";
import type { RouteObject } from "react-router-dom";
import { WorkspacePage } from "@/pages/WorkspacePage/WorkspacePage";

const AgentPage = lazy(() => import("@/pages/AgentPage").then((module) => ({ default: module.AgentPage })));
const ComputerPage = lazy(() => import("@/pages/ComputerPage").then((module) => ({ default: module.ComputerPage })));
const ConversationPage = lazy(() =>
  import("@/pages/ConversationPage").then((module) => ({ default: module.ConversationPage })),
);
const HubPage = lazy(() => import("@/pages/HubPage").then((module) => ({ default: module.HubPage })));
const HumanPage = lazy(() => import("@/pages/HumanPage").then((module) => ({ default: module.HumanPage })));
const ModelProviderPage = lazy(() =>
  import("@/pages/ModelProviderPage/ModelProviderPage").then((module) => ({ default: module.ModelProviderPage })),
);
const SettingsPage = lazy(() => import("@/pages/SettingsPage").then((module) => ({ default: module.SettingsPage })));
const TeamPage = lazy(() => import("@/pages/TeamPage").then((module) => ({ default: module.TeamPage })));
const TasksPage = lazy(() => import("@/pages/TasksPage").then((module) => ({ default: module.TasksPage })));

function routeElement(Page: ComponentType): ReactElement {
  return (
    <Suspense fallback={null}>
      <Page />
    </Suspense>
  );
}

const routes: RouteObject[] = [
  {
    path: "/",
    element: <WorkspacePage />,
    children: [
      { index: true, element: routeElement(ConversationPage) },
      { path: "computer", element: routeElement(ComputerPage) },
      { path: "notifications", element: routeElement(AgentPage) },
      { path: "agents/:agentId", element: routeElement(AgentPage) },
      { path: "agent/:agentId", element: routeElement(AgentPage) },
      { path: "models/:providerId", element: routeElement(ModelProviderPage) },
      { path: "model/:providerId", element: routeElement(ModelProviderPage) },
      { path: "humans/:humanId", element: routeElement(HumanPage) },
      { path: "human/:humanId", element: routeElement(HumanPage) },
      { path: "teams", element: routeElement(TeamPage) },
      { path: "teams/:teamId", element: routeElement(TeamPage) },
      { path: "team/:teamId", element: routeElement(TeamPage) },
      { path: "resources", element: routeElement(HubPage) },
      { path: "hub", element: routeElement(HubPage) },
      { path: "templates/:templateId", element: routeElement(HubPage) },
      { path: "skills/:skillName", element: routeElement(HubPage) },
      { path: "mcp-servers/:mcpName", element: routeElement(HubPage) },
      { path: "tasks", element: routeElement(TasksPage) },
      { path: "tasks/:taskId", element: routeElement(TasksPage) },
      { path: "settings", element: routeElement(SettingsPage) },
      { path: "rooms/:conversationId", element: routeElement(ConversationPage) },
      { path: "room/:conversationId", element: routeElement(ConversationPage) },
      { path: "channels/:conversationId", element: routeElement(ConversationPage) },
      { path: "channel/:conversationId", element: routeElement(ConversationPage) },
      { path: "dms/:conversationId", element: routeElement(ConversationPage) },
      { path: "dm/:conversationId", element: routeElement(ConversationPage) },
      { path: "conversations/:conversationId", element: routeElement(ConversationPage) },
      { path: "conversation/:conversationId", element: routeElement(ConversationPage) },
      { path: "*", element: routeElement(ConversationPage) },
    ],
  },
];

const router = createHashRouter(routes);

export function AppRouter(): ReactElement {
  return <RouterProvider router={router} />;
}
