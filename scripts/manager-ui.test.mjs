import assert from "node:assert/strict";
import { test } from "node:test";
import {
  backendBase,
  createAPI,
  managerUserID,
  managerRooms,
  modelChoices,
  roomModelValue,
  modelConfiguration,
  messageText,
} from "../examples/manager-ui/api.mjs";

test("both/custom deployment bases retain gateway prefixes", () => {
  for (const [input, expected] of [
    ["http://localhost:8080/ui/custom/app.mjs", "http://localhost:8080/"],
    [
      "https://hub.test/instances/42/ui/custom/app.mjs",
      "https://hub.test/instances/42/",
    ],
    ["https://hub.test/instances/42/app.mjs", "https://hub.test/instances/42/"],
    ["http://localhost:8080/app.mjs", "http://localhost:8080/"],
  ])
    assert.equal(backendBase(input).href, expected);
});

test("only the current user's two-party Manager conversations are listed", () => {
  const direct = {
    id: "direct",
    is_direct: true,
    members: ["user-admin", "user-manager"],
  };
  const task = { ...direct, id: "task", task_session: true };
  const rooms = [
    direct,
    task,
    { ...direct, id: "archived", archived: true },
    { ...direct, id: "other-user", members: ["someone", "user-manager"] },
    { ...direct, id: "worker", members: ["user-admin", "worker"] },
    { ...direct, id: "group", is_direct: false },
    {
      ...direct,
      id: "multi",
      members: ["user-admin", "user-manager", "worker"],
    },
  ];
  const bootstrap = {
    current_user_id: "user-admin",
    users: [{ id: "user-manager", role: "manager" }],
    rooms,
  };
  assert.equal(managerUserID(bootstrap), "user-manager");
  assert.deepEqual(
    managerRooms(bootstrap).map((room) => room.id),
    ["direct", "task"],
  );
  assert.deepEqual(managerRooms({ ...bootstrap, users: [] }), []);
});

test("catalog models exclude media-only entries and gate OpenCSG sign-in", () => {
  const catalog = {
    providers: [
      {
        id: "opencsg",
        display_name: "OpenCSG",
        models: ["chat", "chat", "image"],
        image_models: ["image"],
      },
      { id: "local", models: ["local-model"] },
    ],
  };
  const choices = modelChoices(catalog, false);
  assert.equal(choices.length, 2);
  assert.equal(choices[0].disabled, true);
  assert.equal(choices[1].disabled, false);
  assert.equal(modelChoices(catalog, true)[0].disabled, false);
});

test("model selection preserves workspace/execution but clears incompatible reasoning", () => {
  const profile = { model_provider_id: "old", model_id: "old-model" };
  const room = {
    configuration: {
      workspace_dir: "/workspace",
      execution_mode: "approval",
      reasoning_effort: "xhigh",
    },
  };
  assert.equal(
    roomModelValue(room, profile),
    JSON.stringify(["old", "old-model"]),
  );
  const config = modelConfiguration(room, JSON.stringify(["new", "new-model"]));
  assert.deepEqual(config, {
    workspace_dir: "/workspace",
    execution_mode: "approval",
    reasoning_effort: "auto",
    model_provider_id: "new",
    model_id: "new-model",
  });
  assert.equal(room.configuration.reasoning_effort, "xhigh");
  assert.equal(
    roomModelValue({ configuration: config }, profile),
    JSON.stringify(["new", "new-model"]),
  );
});

test("zero-width turn placeholders show progress and runtime failures instead of a blank reply", () => {
  const message = (progress, content = "\u200b") => ({
    content,
    metadata: { csgclaw: { turn_progress: progress } },
  });
  assert.match(messageText(message({ status: "running" })), /正在处理/);
  assert.equal(
    messageText(message({ status: "failed", error: "模型不可用" })),
    "模型不可用",
  );
  assert.match(messageText(message({ status: "failed" })), /回复失败/);
  assert.match(messageText(message({ status: "canceled" })), /已停止/);
  assert.equal(
    messageText(message({ status: "succeeded" }, "双 UI 测试通过")),
    "双 UI 测试通过",
  );
});

test("HTTP transport targets the shared API with JSON and retains structured failures", async () => {
  const requests = [];
  const api = createAPI(
    new URL("https://hub.test/instances/42/"),
    async (url, options) => {
      requests.push({ url: url.href, ...options });
      return new Response(JSON.stringify({ id: "message-1" }), { status: 201 });
    },
  );
  assert.deepEqual(await api("messages", "POST", { content: "你好" }), {
    id: "message-1",
  });
  assert.equal(
    requests[0].url,
    "https://hub.test/instances/42/api/v1/messages",
  );
  assert.equal(requests[0].credentials, "same-origin");
  assert.deepEqual(JSON.parse(requests[0].body), { content: "你好" });
  const fail = createAPI(
    new URL("http://localhost/"),
    async () =>
      new Response(
        JSON.stringify({
          error: { code: "auth_required", message: "请先登录" },
        }),
        { status: 401 },
      ),
  );
  await assert.rejects(
    fail("messages"),
    (error) =>
      error.status === 401 &&
      error.code === "auth_required" &&
      error.message === "请先登录",
  );
  const plain = createAPI(
    new URL("http://localhost/"),
    async () => new Response("runtime unavailable", { status: 503 }),
  );
  await assert.rejects(plain("messages"), /runtime unavailable/);
});
