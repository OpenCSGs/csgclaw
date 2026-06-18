import type { IMMessage } from "@/models/conversations";

export function shouldShowMessageDateDivider(
  previousMessage: IMMessage | null | undefined,
  currentMessage: IMMessage | null | undefined,
): boolean {
  if (!previousMessage) {
    return hasValidMessageTime(currentMessage);
  }
  return !isSameMessageDate(previousMessage, currentMessage);
}

function isSameMessageDate(
  previousMessage: IMMessage | null | undefined,
  currentMessage: IMMessage | null | undefined,
): boolean {
  const previousAt = Date.parse(previousMessage?.created_at || "");
  const currentAt = Date.parse(currentMessage?.created_at || "");
  if (!Number.isFinite(previousAt) || !Number.isFinite(currentAt)) {
    return false;
  }
  const previousDate = new Date(previousAt);
  const currentDate = new Date(currentAt);
  return (
    previousDate.getFullYear() === currentDate.getFullYear() &&
    previousDate.getMonth() === currentDate.getMonth() &&
    previousDate.getDate() === currentDate.getDate()
  );
}

function hasValidMessageTime(message: IMMessage | null | undefined): boolean {
  return Number.isFinite(Date.parse(message?.created_at || ""));
}
