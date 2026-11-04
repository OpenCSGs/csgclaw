import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import vm from "node:vm";
import * as adapters from "../examples/manager-ui/api.mjs";

// Run the actual entrypoint with a small browser harness, without a UI toolchain.
async function startUI() {
  const elements = new Map();
  const element = () => ({
    value: "",
    children: [],
    listeners: {},
    get options() {
      return this.children.flatMap((child) => child.children || [child]);
    },
    append(...children) {
      this.children.push(...children);
    },
    replaceChildren(...children) {
      this.children = children;
    },
    setAttribute() {},
    addEventListener(type, listener) {
      this.listeners[type] = listener;
    },
    querySelectorAll() {
      return this.children;
    },
  });
  const $ = (id) => {
    if (!elements.has(id)) elements.set(id, element());
    return elements.get(id);
  };
  let room = {
    id: "room-1",
    is_direct: true,
    members: ["user-admin", "user-manager"],
    configuration: {
      workspace_dir: "/workspace",
      execution_mode: "standard",
      model_provider_id: "local",
      model_id: "old-model",
    },
  };
  const patches = [];
  let snapshotError = false;
  let holdSnapshot;
  let events;
  let poll;
  const context = vm.createContext({
    ...adapters,
    URL,
    URLSearchParams,
    location: { href: "http://localhost/ui/custom/?room=room-1" },
    history: { replaceState() {} },
    document: { getElementById: $, createElement: element, hidden: false },
    window: { addEventListener() {} },
    Option: class {
      constructor(text, value) {
        this.textContent = text;
        this.value = value;
      }
    },
    EventSource: class {
      constructor() {
        events = this;
      }
    },
    setInterval(callback) {
      poll = callback;
    },
    clearInterval() {},
    createAPI:
      () =>
      async (path, method = "GET", body) => {
        if (path === "auth/status") return { authenticated: true };
        if (path === "bootstrap")
          return {
            current_user_id: "user-admin",
            users: [{ id: "user-manager", role: "manager" }],
            rooms: [structuredClone(room)],
          };
        if (path === "model-providers")
          return {
            providers: [{ id: "local", models: ["old-model", "new-model"] }],
          };
        if (path === "agents/manager/profile") return {};
        if (path.startsWith("messages?")) return [];
        if (path === "rooms" && method === "GET") {
          if (snapshotError) throw new Error("Room snapshot unavailable");
          if (holdSnapshot) {
            const hold = holdSnapshot;
            holdSnapshot = null;
            return hold([structuredClone(room)]);
          }
          return [structuredClone(room)];
        }
        if (path === "rooms/room-1" && method === "PATCH") {
          patches.push(body);
          room.configuration = structuredClone(body.configuration);
          return structuredClone(room);
        }
        throw new Error(`Unexpected API call: ${method} ${path}`);
      },
  });
  const source = await readFile(
    new URL("../examples/manager-ui/app.mjs", import.meta.url),
    "utf8",
  );
  // The real module's adapters are injected above; all app behavior stays intact.
  const script = source
    .replace(/^import \{[\s\S]*?\} from "\.\/api\.mjs";\s*/, "")
    .replaceAll("import.meta.url", '"http://localhost/ui/custom/app.mjs"');
  await vm.runInContext(`(async () => {${script}\n})()`, context);
  const settle = () => new Promise((resolve) => setImmediate(resolve));
  return {
    $,
    patches,
    configure(configuration) {
      room.configuration = { ...room.configuration, ...configuration };
    },
    failSnapshot() {
      snapshotError = true;
    },
    holdSnapshot() {
      let release;
      holdSnapshot = (snapshot) =>
        new Promise((resolve) => {
          release = () => resolve(snapshot);
        });
      return async () => {
        release();
        await settle();
      };
    },
    async event() {
      events.onmessage({
        data: JSON.stringify({ type: "room.updated", room_id: room.id, room }),
      });
      await settle();
    },
    async poll() {
      poll();
      await settle();
    },
    async refresh() {
      $("refresh").listeners.click();
      await settle();
    },
    async selectModel() {
      $("model").value = JSON.stringify(["local", "new-model"]);
      $("model").listeners.change();
      await settle();
    },
  };
}

test("model changes preserve configuration updated in the primary UI without an SSE event", async () => {
  const ui = await startUI();
  ui.configure({
    execution_mode: "read_only",
    workspace_dir: "/updated-workspace",
  });
  await ui.selectModel();
  assert.equal(ui.patches.length, 1);
  assert.equal(ui.patches[0].configuration.execution_mode, "read_only");
  assert.equal(ui.patches[0].configuration.workspace_dir, "/updated-workspace");
  assert.equal(ui.patches[0].configuration.model_id, "new-model");
});

test("room.updated events, polling and manual refresh reconcile the selected model", async () => {
  const ui = await startUI();
  ui.configure({ model_id: "new-model" });
  await ui.event();
  assert.equal(ui.$("model").value, JSON.stringify(["local", "new-model"]));
  ui.configure({ model_id: "old-model" });
  await ui.poll();
  assert.equal(ui.$("model").value, JSON.stringify(["local", "old-model"]));
  ui.configure({ model_id: "new-model" });
  await ui.refresh();
  assert.equal(ui.$("model").value, JSON.stringify(["local", "new-model"]));
});

test("model changes stop without PATCH when the current configuration cannot be fetched", async () => {
  const ui = await startUI();
  ui.failSnapshot();
  await ui.selectModel();
  assert.equal(ui.patches.length, 0);
  assert.match(ui.$("error").textContent, /Room snapshot unavailable/);
});

test("an older poll cannot overwrite a newer room.updated event", async () => {
  const ui = await startUI();
  const release = ui.holdSnapshot();
  await ui.poll();
  ui.configure({ model_id: "new-model" });
  await ui.event();
  await release();
  assert.equal(ui.$("model").value, JSON.stringify(["local", "new-model"]));
});

test("unchanged polling preserves the existing model selector nodes", async () => {
  const ui = await startUI();
  const options = ui.$("model").children;
  await ui.poll();
  assert.equal(ui.$("model").children, options);
});
