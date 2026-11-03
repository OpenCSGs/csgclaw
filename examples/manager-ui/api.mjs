// This frontend is independent of the main UI. Only the public HTTP API is shared.
// Resolve from the module URL so both /ui/custom/ and gateway subpaths work.
export function backendBase(moduleURL) {
  const base = new URL("./", moduleURL);
  if (base.pathname.endsWith("/ui/custom/")) return new URL("../../", base);
  return base;
}

export function requestID() {
  // Idempotency keys are not secrets. Keep remote plain-HTTP demos usable too.
  return (
    globalThis.crypto?.randomUUID?.() ||
    `manager-ui-${Date.now()}-${Math.random().toString(36).slice(2)}`
  );
}

export function managerUserID(bootstrap) {
  return bootstrap.users?.find((user) => user.role === "manager")?.id || "";
}

export function managerRooms(bootstrap) {
  const managerID = managerUserID(bootstrap);
  return (bootstrap.rooms || []).filter(
    (room) =>
      !room.archived &&
      room.is_direct &&
      room.members?.length === 2 &&
      room.members.includes(bootstrap.current_user_id) &&
      room.members.includes(managerID),
  );
}

export function modelChoices(catalog, authenticated) {
  return (catalog.providers || []).flatMap((provider) => {
    const media = new Set([
      ...(provider.image_models || []),
      ...(provider.video_models || []),
    ]);
    return [...new Set(provider.models || [])]
      .filter((id) => !media.has(id))
      .map((id) => ({
        value: JSON.stringify([provider.id, id]),
        label: id,
        provider: provider.display_name || provider.id,
        disabled: provider.id === "opencsg" && !authenticated,
      }));
  });
}

export function roomModelValue(room, profile) {
  return JSON.stringify([
    room?.configuration?.model_provider_id || profile?.model_provider_id || "",
    room?.configuration?.model_id || profile?.model_id || "",
  ]);
}

export function modelConfiguration(room, value) {
  const [provider, model] = JSON.parse(value);
  if (!provider || !model) throw new Error("请选择可用模型");
  // Preserve the locked workspace and execution mode; model credentials stay on the server.
  return {
    ...room.configuration,
    model_provider_id: provider,
    model_id: model,
    reasoning_effort: "auto",
  };
}

export function messageText(message) {
  const content = (message.content || "").replace(/\u200b/g, "").trim();
  if (content) return content;
  const progress = message.metadata?.csgclaw?.turn_progress;
  if (progress?.error) return progress.error;
  if (progress?.status === "failed")
    return "Manager 回复失败，请切换模型或稍后重试。";
  if (progress?.status === "succeeded") return "本轮已完成（无文本回复）。";
  if (
    ["stopped", "canceled", "cancelled", "interrupted"].includes(
      progress?.status,
    )
  )
    return "本轮已停止。";
  if (progress) return "Manager 正在处理…";
  return message.attachments?.length
    ? "[附件，请在完整 UI 中查看]"
    : "[会话事件]";
}

export function createAPI(base, fetcher = globalThis.fetch) {
  return async function api(path, method = "GET", body) {
    const response = await fetcher(new URL(`api/v1/${path}`, base), {
      method,
      credentials: "same-origin",
      headers: {
        Accept: "application/json",
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await response.text();
    let data;
    try {
      data = text ? JSON.parse(text) : undefined;
    } catch {
      /* Legacy errors may be plain text. */
    }
    if (!response.ok) {
      const error = new Error(
        data?.error?.message ||
          (typeof data?.error === "string" ? data.error : "") ||
          text ||
          `请求失败 (${response.status})`,
      );
      error.status = response.status;
      error.code = data?.error?.code;
      throw error;
    }
    if (text && data === undefined) throw new Error("后端返回了无效的 JSON");
    return data;
  };
}
