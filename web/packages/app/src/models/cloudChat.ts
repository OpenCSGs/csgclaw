export type CloudChatResponse = {
  id: string;
  status: "queued" | "in_progress" | "completed" | "failed" | "cancelled";
  model: string;
  output?: { content: { type: string; text: string }[] }[];
  error?: { code: string; message: string } | null;
};
export type CloudChatTurn = {
  key: string;
  input: string;
  previousResponseID: string;
  responseID?: string;
  submittedAt: number;
};

export type CloudChatState = {
  messages: { id: string; role: "user" | "assistant"; text: string }[];
  previousResponseID: string;
  pending: CloudChatTurn | null;
};
export function emptyCloudChat(): CloudChatState {
  return { messages: [], previousResponseID: "", pending: null };
}
export function parseCloudChat(raw: string | null): CloudChatState {
  try {
    const value = JSON.parse(raw || "null") as Partial<CloudChatState> | null;
    if (!value || !Array.isArray(value.messages) || typeof value.previousResponseID !== "string")
      return emptyCloudChat();
    const messages = value.messages
      .filter(
        (message) =>
          message &&
          typeof message.id === "string" &&
          ["user", "assistant"].includes(message.role) &&
          typeof message.text === "string",
      )
      .map(({ id, role, text }) => ({ id, role, text }));
    const p = value.pending;
    const pending =
      p &&
      typeof p.key === "string" &&
      typeof p.input === "string" &&
      typeof p.previousResponseID === "string" &&
      (p.responseID === undefined || typeof p.responseID === "string")
        ? {
            key: p.key,
            input: p.input,
            previousResponseID: p.previousResponseID,
            responseID: p.responseID,
            submittedAt: typeof p.submittedAt === "number" ? p.submittedAt : 0,
          }
        : null;
    return { messages, previousResponseID: value.previousResponseID, pending };
  } catch {
    return emptyCloudChat();
  }
}
export function cloudResponseText(response: CloudChatResponse): string {
  return (response.output || [])
    .flatMap((item) => item.content)
    .filter((part) => part.type === "output_text")
    .map((part) => part.text)
    .join("");
}
