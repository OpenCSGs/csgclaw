import {
  backendBase,
  requestID,
  createAPI,
  managerUserID,
  managerRooms,
  modelChoices,
  roomModelValue,
  modelConfiguration,
  messageText,
} from "./api.mjs";

const base = backendBase(import.meta.url);
const api = createAPI(base);
const $ = (id) => document.getElementById(id);
const state = {
  auth: {},
  bootstrap: {},
  rooms: [],
  room: null,
  profile: {},
  catalog: {},
  busy: false,
  sending: false,
  messagesKey: "",
};
let messagesRequest = 0;
let refreshRequest = 0;
let roomRequest = 0;
let events;
let poll;

$("primary-ui").href = base.href;

function showError(error) {
  $("error").textContent = error?.message || "";
  $("error").hidden = !error;
}

function notice(text) {
  $("notice").textContent = text;
  $("notice").hidden = !text;
}

function updateControls() {
  const disabled = state.busy || state.sending || !state.room;
  $("input").disabled = disabled;
  $("send").disabled = disabled || !$("input").value.trim();
  $("model").disabled = disabled || !$("model").options.length;
  $("new-chat").disabled =
    state.busy ||
    state.sending ||
    !state.bootstrap.current_user_id ||
    !managerUserID(state.bootstrap);
  $("refresh").disabled = state.busy || state.sending;
  $("auth").disabled = state.busy || state.sending;
  $("send-status").textContent = state.sending
    ? "正在发送…"
    : "Enter 发送 · Shift + Enter 换行";
  for (const button of $("rooms").querySelectorAll("button"))
    button.disabled = state.busy || state.sending;
}

function renderRooms() {
  $("rooms").replaceChildren(
    ...state.rooms.map((room) => {
      const button = document.createElement("button");
      button.className = `room${room.id === state.room?.id ? " active" : ""}`;
      button.textContent = room.task_session ? room.title : "Manager 默认会话";
      button.title = button.textContent;
      if (room.id === state.room?.id)
        button.setAttribute("aria-current", "page");
      button.addEventListener(
        "click",
        () =>
          void action(async () => {
            selectRoom(room);
            await refreshMessages();
          }),
      );
      return button;
    }),
  );
}

function renderModels() {
  const selected = roomModelValue(state.room, state.profile);
  const groups = new Map();
  for (const choice of modelChoices(state.catalog, state.auth.authenticated)) {
    if (!groups.has(choice.provider)) {
      const group = document.createElement("optgroup");
      group.label = choice.provider;
      groups.set(choice.provider, group);
    }
    const option = new Option(choice.label, choice.value);
    option.disabled = choice.disabled;
    groups.get(choice.provider).append(option);
  }
  const placeholder = new Option("选择模型", "", true, true);
  placeholder.disabled = true;
  $("model").replaceChildren(placeholder, ...groups.values());
  if (Array.from($("model").options).some((item) => item.value === selected))
    $("model").value = selected;
  else if (state.profile.model_id)
    placeholder.textContent = `当前：${state.room?.configuration?.model_id || state.profile.model_id}`;
  if (!groups.size)
    placeholder.textContent = "暂无模型，请先登录或配置模型服务";
}

function renderMessages(messages) {
  // Plain text is intentional: model output never becomes executable HTML.
  const key = JSON.stringify(messages);
  if (key === state.messagesKey) return;
  state.messagesKey = key;
  const nearBottom =
    $("messages").scrollHeight -
      $("messages").scrollTop -
      $("messages").clientHeight <
    100;
  const nodes = messages.map((message) => {
    const own = message.sender_id === state.bootstrap.current_user_id;
    const row = document.createElement("article");
    row.className = `message ${own ? "own" : "assistant"}`;
    const avatar = document.createElement("span");
    avatar.className = `avatar ${own ? "" : "bot"}`;
    avatar.textContent = own ? "U" : "M";
    avatar.setAttribute("aria-hidden", "true");
    const body = document.createElement("div");
    const name = document.createElement("strong");
    name.textContent =
      message.kind === "event" ? "系统" : own ? "你" : "Manager";
    const text = document.createElement("div");
    text.className = "message-text";
    text.textContent = messageText(message);
    body.append(name, text);
    row.append(avatar, body);
    return row;
  });
  if (!nodes.length) {
    const empty = document.createElement("div");
    empty.className = "empty";
    const title = document.createElement("h2");
    title.textContent = "今天，有什么可以帮你？";
    const text = document.createElement("p");
    text.textContent = "选择模型，开始与 Manager 对话。";
    empty.append(title, text);
    nodes.push(empty);
  }
  $("messages").replaceChildren(...nodes);
  if (nearBottom || messages.length < 2)
    $("messages").scrollTop = $("messages").scrollHeight;
}

function selectRoom(room) {
  ++roomRequest;
  state.room = room;
  state.messagesKey = "";
  const url = new URL(location.href);
  url.searchParams.set("room", room.id);
  history.replaceState(null, "", url);
  renderRooms();
  renderModels();
  renderMessages([]);
}

function reconcileRoom(room) {
  const previous =
    state.room?.id === room.id
      ? state.room
      : state.rooms.find((item) => item.id === room.id);
  if (state.room?.id === room.id) ++roomRequest;
  // Unchanged polls should not rebuild a model selector the user has open.
  if (JSON.stringify(previous) === JSON.stringify(room)) return;
  state.rooms = state.rooms.map((item) => (item.id === room.id ? room : item));
  if (state.room?.id === room.id) {
    state.room = room;
    renderModels();
  }
  renderRooms();
  updateControls();
}

async function refreshRoom() {
  const roomID = state.room?.id;
  if (!roomID) return;
  // An SSE update or room switch invalidates an older poll response.
  const request = ++roomRequest;
  const rooms = await api("rooms");
  const room = rooms.find((item) => item.id === roomID);
  if (request === roomRequest && state.room?.id === roomID && room)
    reconcileRoom(room);
}

async function refreshConversation() {
  await Promise.all([refreshRoom(), refreshMessages()]);
}

async function refreshMessages() {
  const roomID = state.room?.id;
  if (!roomID) return;
  const request = ++messagesRequest;
  const messages = await api(
    `messages?${new URLSearchParams({ room_id: roomID })}`,
  );
  if (request === messagesRequest && state.room?.id === roomID)
    renderMessages(messages || []);
}

async function refresh() {
  const request = ++refreshRequest;
  const [auth, bootstrap, catalog, profile] = await Promise.all([
    api("auth/status"),
    api("bootstrap"),
    api("model-providers"),
    api("agents/manager/profile"),
  ]);
  if (request !== refreshRequest) return;
  Object.assign(state, {
    auth,
    bootstrap,
    catalog,
    profile,
    rooms: managerRooms(bootstrap),
  });
  $("account-name").textContent = auth.authenticated
    ? auth.name || auth.user_id || "已登录"
    : "未登录";
  $("account-detail").textContent = auth.authenticated
    ? "OpenCSG 已连接"
    : "OpenCSG 账号";
  $("auth").textContent = auth.authenticated ? "退出" : "登录";
  const roomID = new URL(location.href).searchParams.get("room");
  const room = state.rooms.find((item) => item.id === roomID) || state.rooms[0];
  if (room) {
    if (state.room?.id !== room.id) selectRoom(room);
    else reconcileRoom(room);
    await refreshMessages();
  } else {
    state.room = null;
    renderRooms();
    renderModels();
    renderMessages([]);
  }
  notice(
    auth.authenticated
      ? ""
      : "登录 OpenCSG 后可使用平台模型；已配置的其他模型服务仍可使用。",
  );
  $("messages").setAttribute("aria-busy", "false");
}

async function action(fn) {
  if (state.busy || state.sending) return;
  state.busy = true;
  showError(null);
  updateControls();
  try {
    await fn();
  } catch (error) {
    showError(error);
  } finally {
    state.busy = false;
    updateControls();
  }
}

$("refresh").addEventListener("click", () => void action(refresh));
$("input").addEventListener("input", updateControls);
$("input").addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    $("composer").requestSubmit();
  }
});
$("auth").addEventListener(
  "click",
  () =>
    void action(async () => {
      if (state.auth.authenticated) {
        await api("auth/logout", "POST", {});
        await refresh();
        return;
      }
      const returnURL = new URL(location.href);
      returnURL.searchParams.delete("auth_result");
      returnURL.searchParams.delete("auth_reason");
      const result = await api("auth/login", "POST", {
        return_url: returnURL.href,
      });
      const loginURL = new URL(result.login_url);
      if (!["https:", "http:"].includes(loginURL.protocol))
        throw new Error("登录地址无效");
      location.assign(loginURL.href);
    }),
);
$("model").addEventListener(
  "change",
  () =>
    void action(async () => {
      const roomID = state.room.id;
      const value = $("model").value;
      try {
        // Configuration is replaced by PATCH. Read it now so another UI's
        // execution mode/workspace changes survive even if an SSE event was lost.
        const rooms = await api("rooms");
        const room = rooms.find((item) => item.id === roomID);
        if (!room) throw new Error("会话已不存在，请刷新后重试。");
        reconcileRoom(room);
        const updated = await api(
          `rooms/${encodeURIComponent(roomID)}`,
          "PATCH",
          {
            configuration: modelConfiguration(room, value),
          },
        );
        reconcileRoom(updated);
      } finally {
        renderModels();
      }
    }),
);
$("new-chat").addEventListener(
  "click",
  () =>
    void action(async () => {
      const room = await api("rooms", "POST", {
        title: `新会话 ${new Date().toLocaleString("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" })}`,
        type: "free",
        is_direct: true,
        task_session: true,
        locale: "zh-CN",
        creator_id: state.bootstrap.current_user_id,
        member_ids: [managerUserID(state.bootstrap)],
        client_request_id: requestID(),
      });
      state.rooms.unshift(room);
      selectRoom(room);
      await refreshMessages();
    }),
);
$("composer").addEventListener("submit", async (event) => {
  event.preventDefault();
  const content = $("input").value.trim();
  if (!content || !state.room || state.busy || state.sending) return;
  state.sending = true;
  showError(null);
  updateControls();
  try {
    await api("messages", "POST", {
      room_id: state.room.id,
      sender_id: state.bootstrap.current_user_id,
      content,
      locale: "zh-CN",
      client_message_id: requestID(),
    });
    $("input").value = "";
    await refreshMessages();
    $("messages").scrollTop = $("messages").scrollHeight;
  } catch (error) {
    showError(error);
  } finally {
    state.sending = false;
    updateControls();
    $("input").focus();
  }
});

// SSE triggers snapshots; periodic recovery also covers missed events and reconnects.
// There is no second agent runtime or model credential stored in this frontend.
function connectEvents() {
  events = new EventSource(new URL("api/v1/events", base));
  events.onopen = () => {
    $("connection").textContent = "已连接 · 专属智能助手";
    void refreshConversation().catch(showError);
  };
  events.onerror = () => {
    $("connection").textContent = "正在重连 · 自动同步中";
  };
  events.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === "room.updated" && data.room?.id)
        reconcileRoom(data.room);
      if (data.room_id === state.room?.id)
        void refreshMessages().catch(showError);
    } catch {
      /* Ignore unknown event envelopes. */
    }
  };
  poll = setInterval(() => {
    if (!document.hidden && !state.busy && !state.sending)
      void refreshConversation().catch(showError);
  }, 2000);
}

const callback = new URL(location.href);
const loginFailed = callback.searchParams.get("auth_result") === "failed";
callback.searchParams.delete("auth_result");
callback.searchParams.delete("auth_reason");
history.replaceState(null, "", callback);
await action(refresh);
if (loginFailed) showError(new Error("登录未完成，请重试。"));
connectEvents();
window.addEventListener("pagehide", () => {
  events?.close();
  clearInterval(poll);
});
window.addEventListener("pageshow", (event) => {
  if (event.persisted) {
    connectEvents();
    void action(refresh);
  }
});
