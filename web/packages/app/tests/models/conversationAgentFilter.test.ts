import {
  collaborationConversationOwner,
  conversationDirectory,
  conversationFilterOptions,
  conversationOwner,
} from "@/models/conversationAgentFilter";
import type { IMConversation } from "@/models/conversations";

const direct: IMConversation = { id: "direct", members: ["admin", "worker"], is_direct: true, messages: [] };
const cloud = {
  ...direct,
  id: "cloud",
  is_direct: false,
  cloud_agent: { user_id: "cloud-worker", agent_id: "agent", connection_id: "connection" },
};
const group = { ...direct, id: "group", is_direct: false, members: ["admin", "worker", "another-worker"] };
const directories = new Map([["user-worker", "/default"]]);

it("assigns one owner with cloud precedence and matches local account aliases", () => {
  expect(conversationOwner(cloud, "user-admin")).toBe("cloud:cloud-worker");
  expect(conversationOwner(direct, "user-admin")).toBe("direct:user-worker");
  expect(conversationOwner(group, "user-admin")).toBe(collaborationConversationOwner);
  expect(conversationOwner({ ...direct, members: ["user-admin"] }, "admin")).toBe("");
});

it("takes the recorded directory and limits the available directories by owner", () => {
  const task = { ...direct, id: "task", configuration: { workspace_dir: "/task" } };
  const space = { ...group, configuration: { workspace_dir: "/space" } };
  const otherSpace = { ...group, configuration: { workspace_dir: "/other-space" } };
  expect(conversationDirectory(direct, "user-admin", directories)).toBe("/default");
  expect(conversationDirectory({ ...direct, configuration: {} }, "user-admin", directories)).toBe("");
  expect(conversationDirectory(group, "user-admin", directories)).toBe("");
  const options = conversationFilterOptions(
    [direct, task, space, otherSpace, cloud],
    { owner: "direct:user-worker", directory: "/space" },
    "user-admin",
    new Map(),
    directories,
    (key) => key,
  );
  expect(options.directories).toEqual([
    { path: "/default", count: 1 },
    { path: "/space", count: 0 },
    { path: "/task", count: 1 },
  ]);
  expect(options.owners.map(({ value, count }) => [value, count])).toEqual([
    ["collaboration", 1],
    ["direct:user-worker", 0],
  ]);
  expect(options.ownerTotal).toBe(1);
  expect(options.directoryTotal).toBe(2);
});

it("preserves selected values for recovery when a search returns no matching groups", () => {
  const options = conversationFilterOptions(
    [],
    { owner: "cloud:cloud-worker", directory: "/task" },
    "user-admin",
    new Map(),
    directories,
    (key) => key,
  );
  expect(options.owners).toMatchObject([{ value: "cloud:cloud-worker", label: "cloud-worker", count: 0 }]);
  expect(options.directories).toEqual([{ path: "/task", count: 0 }]);
  expect(options.ownerTotal).toBe(0);
  expect(options.directoryTotal).toBe(0);
});
