import { lazy, Suspense } from "react";
import type { ComponentType, ReactElement } from "react";
import { createHashRouter, RouterProvider, Navigate, useLocation } from "react-router-dom";
import { settingsDialogSearch } from "@/models/workspaceDialogs";
import type { RouteObject } from "react-router-dom";
import { WorkspacePage } from "@/pages/WorkspacePage/WorkspacePage";
import { loadRouteModule } from "./loadRouteModule";

const StartTaskPage = lazy(() =>
  loadRouteModule(() =>
    import("@/pages/StartTaskPage/StartTaskPage").then((module) => ({ default: module.StartTaskPage })),
  ),
);
const AgentOverviewPage = lazy(() =>
  loadRouteModule(() =>
    import("@/pages/AgentOverviewPage/AgentOverviewPage").then((module) => ({ default: module.AgentOverviewPage })),
  ),
);

const AppsPage = lazy(() =>
  loadRouteModule(() => import("@/pages/AppsPage/AppsPage").then((module) => ({ default: module.AppsPage }))),
);

const AgentPage = lazy(() =>
  loadRouteModule(() => import("@/pages/AgentPage").then((module) => ({ default: module.AgentPage }))),
);
const ComputerPage = lazy(() =>
  loadRouteModule(() => import("@/pages/ComputerPage").then((module) => ({ default: module.ComputerPage }))),
);
const ConversationPage = lazy(() =>
  loadRouteModule(() => import("@/pages/ConversationPage").then((module) => ({ default: module.ConversationPage }))),
);
const HubPage = lazy(() =>
  loadRouteModule(() => import("@/pages/HubPage").then((module) => ({ default: module.HubPage }))),
);
const HumanPage = lazy(() =>
  loadRouteModule(() => import("@/pages/HumanPage").then((module) => ({ default: module.HumanPage }))),
);
const ModelProviderPage = lazy(() =>
  loadRouteModule(() =>
    import("@/pages/ModelProviderPage/ModelProviderPage").then((module) => ({ default: module.ModelProviderPage })),
  ),
);
const SessionDemoPage = lazy(() =>
  loadRouteModule(() => import("@/pages/SessionDemoPage").then((module) => ({ default: module.SessionDemoPage }))),
);

const TasksPage = lazy(() =>
  loadRouteModule(() => import("@/pages/TasksPage").then((module) => ({ default: module.TasksPage }))),
);

function routeElement(Page: ComponentType): ReactElement {
  return (
    <Suspense fallback={null}>
      <Page />
    </Suspense>
  );
}

function SettingsRedirect() {
  const location = useLocation();
  return <Navigate replace to={{ pathname: "/", search: settingsDialogSearch(location.search) }} />;
}

const routes: RouteObject[] = [
  { path: "/session-demo", element: routeElement(SessionDemoPage) },
  {
    path: "/",
    element: <WorkspacePage />,
    children: [
      { index: true, element: routeElement(StartTaskPage) },
      { path: "new", element: routeElement(StartTaskPage) },
      { path: "agents", element: routeElement(AgentOverviewPage) },
      { path: "computer", element: routeElement(ComputerPage) },
      { path: "notifications", element: routeElement(AgentPage) },
      { path: "agents/:agentId", element: routeElement(AgentOverviewPage) },
      { path: "agent/:agentId", element: routeElement(AgentOverviewPage) },
      { path: "models/:providerId", element: routeElement(ModelProviderPage) },
      { path: "model/:providerId", element: routeElement(ModelProviderPage) },
      { path: "humans/:humanId", element: routeElement(HumanPage) },
      { path: "human/:humanId", element: routeElement(HumanPage) },
      { path: "teams", element: routeElement(AgentOverviewPage) },
      { path: "teams/:teamId", element: routeElement(AgentOverviewPage) },
      { path: "team/:teamId", element: routeElement(AgentOverviewPage) },
      { path: "connectors", element: routeElement(AppsPage) },
      { path: "connectors/:resourceId", element: routeElement(AppsPage) },
      { path: "resources", element: routeElement(HubPage) },
      { path: "hub", element: routeElement(HubPage) },
      { path: "templates/:templateId", element: routeElement(HubPage) },
      { path: "skills/:skillName", element: routeElement(HubPage) },
      { path: "mcp-servers/:mcpName", element: routeElement(HubPage) },
      { path: "knowledge-bases", element: routeElement(HubPage) },
      { path: "knowledge-bases/:knowledgeBaseId", element: routeElement(HubPage) },
      { path: "scheduled-tasks", element: routeElement(TasksPage) },
      { path: "tasks", element: <Navigate replace to="/" /> },
      { path: "tasks/:taskId", element: routeElement(TasksPage) },
      { path: "settings", element: <SettingsRedirect /> },
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
