import { useCallback, useEffect, useLayoutEffect, useRef, type RefObject } from "react";
import type { IMMessage } from "@/models/conversations";
import { MESSAGE_LIST_BOTTOM_THRESHOLD } from "@/shared/constants/workspace";

type MessageListScrollBehavior = "auto" | "smooth";

export type UseMessageListAutoScrollArgs = {
  active: boolean;
  conversationId: string;
  messageListRef: RefObject<HTMLElement | null>;
  visibleMessagesKey: string;
};

export type MessageListAutoScrollController = {
  follow: (behavior?: MessageListScrollBehavior) => void;
};

function scrollMessageListToBottom(el: HTMLElement, behavior: MessageListScrollBehavior = "auto"): void {
  const top = el.scrollHeight;
  if (behavior === "smooth" && typeof el.scrollTo === "function") {
    el.scrollTo({ top, behavior: "smooth" });
    return;
  }
  el.scrollTop = top;
}

function requestBrowserAnimationFrame(callback: () => void): number | null {
  if (typeof window === "undefined" || typeof window.requestAnimationFrame !== "function") {
    callback();
    return null;
  }
  return window.requestAnimationFrame(callback);
}

function cancelBrowserAnimationFrame(frame: number | null): void {
  if (frame === null || typeof window === "undefined" || typeof window.cancelAnimationFrame !== "function") {
    return;
  }
  window.cancelAnimationFrame(frame);
}

function messageScrollKey(message: IMMessage, index: number): string {
  return [
    message.id || index,
    message.created_at || "",
    message.sender_id || "",
    message.kind || "",
    message.content || "",
    message.thread?.reply_count ?? "",
    message.thread?.latest_reply?.id || "",
    message.thread?.latest_reply?.created_at || "",
    message.thread?.latest_reply?.content || "",
  ].join("\u0001");
}

export function messageListScrollKey(messages: readonly IMMessage[]): string {
  return messages.map((message, index) => messageScrollKey(message, index)).join("\u0002");
}

export function useMessageListAutoScroll({
  active,
  conversationId,
  messageListRef,
  visibleMessagesKey,
}: UseMessageListAutoScrollArgs): MessageListAutoScrollController {
  const shouldAutoScrollRef = useRef(true);
  const autoScrollConversationRef = useRef(conversationId);
  const autoScrollStateFrameRef = useRef<number | null>(null);
  const messageListResizeObserverRef = useRef<ResizeObserver | null>(null);
  const messageListMutationObserverRef = useRef<MutationObserver | null>(null);
  const messageListScrollFrameRef = useRef<number | null>(null);
  const observedMessageListRef = useRef<HTMLElement | null>(null);

  const updateAutoScrollState = useCallback(() => {
    const el = observedMessageListRef.current;
    if (!el) {
      return;
    }
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    shouldAutoScrollRef.current = distanceFromBottom <= MESSAGE_LIST_BOTTOM_THRESHOLD;
  }, []);

  const scheduleAutoScrollStateUpdate = useCallback(() => {
    if (autoScrollStateFrameRef.current !== null) {
      return;
    }
    autoScrollStateFrameRef.current = requestBrowserAnimationFrame(() => {
      autoScrollStateFrameRef.current = null;
      updateAutoScrollState();
    });
  }, [updateAutoScrollState]);

  const scrollToBottomAfterLayout = useCallback(
    (behavior: MessageListScrollBehavior = "auto") => {
      const scroll = () => {
        const el = messageListRef.current;
        if (!el) {
          return;
        }
        scrollMessageListToBottom(el, behavior);
      };
      cancelBrowserAnimationFrame(messageListScrollFrameRef.current);
      messageListScrollFrameRef.current = null;
      scroll();
      messageListScrollFrameRef.current = requestBrowserAnimationFrame(() => {
        messageListScrollFrameRef.current = null;
        scroll();
      });
    },
    [messageListRef],
  );

  const disconnectMessageListElement = useCallback(() => {
    const observed = observedMessageListRef.current;
    if (observed) {
      observed.removeEventListener("scroll", scheduleAutoScrollStateUpdate);
    }
    messageListResizeObserverRef.current?.disconnect();
    messageListMutationObserverRef.current?.disconnect();
    cancelBrowserAnimationFrame(autoScrollStateFrameRef.current);
    cancelBrowserAnimationFrame(messageListScrollFrameRef.current);
    observedMessageListRef.current = null;
    messageListResizeObserverRef.current = null;
    messageListMutationObserverRef.current = null;
    autoScrollStateFrameRef.current = null;
    messageListScrollFrameRef.current = null;
  }, [scheduleAutoScrollStateUpdate]);

  useLayoutEffect(() => {
    const nextElement = active ? messageListRef.current : null;
    if (observedMessageListRef.current === nextElement) {
      return;
    }
    disconnectMessageListElement();
    if (!nextElement) {
      return;
    }

    observedMessageListRef.current = nextElement;
    updateAutoScrollState();
    nextElement.addEventListener("scroll", scheduleAutoScrollStateUpdate, { passive: true });

    if (typeof ResizeObserver === "function") {
      const resizeObserver = new ResizeObserver(() => {
        if (shouldAutoScrollRef.current) {
          scrollToBottomAfterLayout();
        }
      });
      const observeMessageListContent = () => {
        resizeObserver.disconnect();
        resizeObserver.observe(nextElement);
        Array.from(nextElement.children).forEach((child) => resizeObserver.observe(child));
      };
      observeMessageListContent();
      messageListResizeObserverRef.current = resizeObserver;

      if (typeof MutationObserver === "function") {
        const mutationObserver = new MutationObserver(observeMessageListContent);
        mutationObserver.observe(nextElement, { childList: true });
        messageListMutationObserverRef.current = mutationObserver;
      }
    }
  });

  useEffect(() => () => disconnectMessageListElement(), [disconnectMessageListElement]);

  const follow = useCallback(
    (behavior: MessageListScrollBehavior = "auto") => {
      shouldAutoScrollRef.current = true;
      scrollToBottomAfterLayout(behavior);
    },
    [scrollToBottomAfterLayout],
  );

  useLayoutEffect(() => {
    if (!active || !conversationId) {
      return;
    }
    autoScrollConversationRef.current = conversationId;
    follow();
  }, [active, conversationId, follow]);

  useEffect(() => {
    if (!active) {
      return;
    }
    if (autoScrollConversationRef.current !== conversationId) {
      autoScrollConversationRef.current = conversationId;
      shouldAutoScrollRef.current = false;
      return;
    }
    if (!messageListRef.current || !shouldAutoScrollRef.current) {
      return;
    }
    scrollToBottomAfterLayout("smooth");
  }, [active, conversationId, messageListRef, scrollToBottomAfterLayout, visibleMessagesKey]);

  return { follow };
}
