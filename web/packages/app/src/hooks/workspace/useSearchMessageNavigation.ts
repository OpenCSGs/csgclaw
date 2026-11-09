import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { isThreadReply, type IMMessage } from "@/models/conversations";
import { MESSAGE_LIST_NAVIGATE_EVENT } from "@/shared/constants/workspace";

type SearchTarget = { roomID: string; message: IMMessage };

export function useSearchMessageNavigation({
  active,
  roomID,
  messageListRef,
  openThread,
}: {
  active: boolean;
  roomID: string;
  messageListRef: RefObject<HTMLElement | null>;
  openThread: (rootID: string) => Promise<void>;
}) {
  const [target, setTarget] = useState<SearchTarget | null>(null);
  const openThreadRef = useRef(openThread);
  openThreadRef.current = openThread;

  const navigateToSearchMatch = useCallback((nextRoomID: string, message: IMMessage) => {
    if (nextRoomID && message.id) setTarget({ roomID: nextRoomID, message });
  }, []);

  useEffect(() => {
    if (!target || !active || target.roomID !== roomID) return;
    let cancelled = false;
    let frame = 0;
    const isReply = isThreadReply(target.message);

    const locate = () => {
      const deadline = performance.now() + 10000;
      const attempt = () => {
        if (cancelled) return;
        const scroll = isReply ? document.querySelector<HTMLElement>(".thread-panel-body") : messageListRef.current;
        const message = scroll
          ? Array.from(scroll.querySelectorAll<HTMLElement>("[data-message-id]")).find(
              (element) => element.dataset.messageId === target.message.id,
            )
          : undefined;
        if (scroll && message) {
          scroll.dispatchEvent(new Event(MESSAGE_LIST_NAVIGATE_EVENT));
          message.scrollIntoView({ block: "center", behavior: "auto" });
          const bubble = message.querySelector<HTMLElement>(".message-bubble, .thread-message-bubble") || message;
          bubble.animate?.(
            [{ outline: "2px solid var(--primary)", outlineOffset: "3px" }, { outline: "2px solid transparent" }],
            { duration: 1800 },
          );
          setTarget(null);
          return;
        }
        if (performance.now() < deadline) frame = requestAnimationFrame(attempt);
        else setTarget(null);
      };
      frame = requestAnimationFrame(attempt);
    };

    const rootID = isReply ? target.message.relates_to?.event_id : undefined;
    if (rootID) void openThreadRef.current(rootID).finally(() => !cancelled && locate());
    else locate();
    return () => {
      cancelled = true;
      cancelAnimationFrame(frame);
    };
  }, [active, roomID, messageListRef, target]);

  return navigateToSearchMatch;
}
