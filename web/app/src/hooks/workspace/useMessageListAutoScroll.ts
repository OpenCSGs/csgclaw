import { parseTurnProgress } from "@/models/turnProgress";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import type { IMMessage } from "@/models/conversations";
import { MESSAGE_LIST_BOTTOM_THRESHOLD, MESSAGE_LIST_NAVIGATE_EVENT } from "@/shared/constants/workspace";

type MessageListScrollBehavior = "auto" | "smooth";

export type UseMessageListAutoScrollArgs = {
  active: boolean;
  conversationId: string;
  deferNewMessageNotice?: boolean;
  latestMessageIsReply?: boolean;
  messageListRef: RefObject<HTMLElement | null>;
  waitingForReply?: boolean;
  visibleMessagesKey: string;
};

export type MessageListAutoScrollController = {
  follow: (behavior?: MessageListScrollBehavior) => void;
  hasNewMessages: boolean;
  preserveAnchor: (anchor?: HTMLElement | null) => void;
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
    parseTurnProgress(message)?.revision ?? "",
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
  deferNewMessageNotice = false,
  latestMessageIsReply = false,
  messageListRef,
  waitingForReply = false,
  visibleMessagesKey,
}: UseMessageListAutoScrollArgs): MessageListAutoScrollController {
  const [hasNewMessages, setHasNewMessages] = useState(false);
  const hasPendingNewReplyRef = useRef(false);
  const shouldAutoScrollRef = useRef(true);
  const previousConversationRef = useRef(conversationId);
  const previousMessagesKeyRef = useRef(visibleMessagesKey);
  const pendingInitialFollowRef = useRef(active && Boolean(conversationId));
  const autoScrollConversationRef = useRef(conversationId);
  const autoScrollStateFrameRef = useRef<number | null>(null);
  const messageListResizeObserverRef = useRef<ResizeObserver | null>(null);
  const messageListMutationObserverRef = useRef<MutationObserver | null>(null);
  const messageListScrollFrameRef = useRef<number | null>(null);
  const messageListAnchorFrameRef = useRef<number | null>(null);
  const messageListAnchorVersionRef = useRef(0);
  const preservingAnchorRef = useRef(false);
  const observedMessageListRef = useRef<HTMLElement | null>(null);

  const pauseOnUpwardWheel = useCallback((event: WheelEvent) => {
    if (event.deltaY < 0) shouldAutoScrollRef.current = false;
  }, []);

  const handleMessageNavigation = useCallback((event: Event) => {
    shouldAutoScrollRef.current = event instanceof CustomEvent && event.detail === "follow";
    if (shouldAutoScrollRef.current) {
      hasPendingNewReplyRef.current = false;
      setHasNewMessages(false);
    }
    pendingInitialFollowRef.current = false;
    preservingAnchorRef.current = false;
    messageListAnchorVersionRef.current += 1;
    cancelBrowserAnimationFrame(messageListAnchorFrameRef.current);
    messageListAnchorFrameRef.current = null;
    cancelBrowserAnimationFrame(messageListScrollFrameRef.current);
    messageListScrollFrameRef.current = null;
  }, []);

  const updateAutoScrollState = useCallback(() => {
    const el = observedMessageListRef.current;
    if (!el) {
      return;
    }
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    shouldAutoScrollRef.current = distanceFromBottom <= MESSAGE_LIST_BOTTOM_THRESHOLD;
    if (shouldAutoScrollRef.current) {
      hasPendingNewReplyRef.current = false;
      setHasNewMessages(false);
    }
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
      if (!messageListRef.current) {
        return false;
      }
      const scroll = () => {
        const el = messageListRef.current;
        if (!el || !shouldAutoScrollRef.current || el.querySelector('[data-preserve-scroll="true"]')) {
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
      return true;
    },
    [messageListRef],
  );

  const preserveAnchor = useCallback(
    (anchor?: HTMLElement | null) => {
      const el = messageListRef.current;
      if (!el) {
        return;
      }

      const initialScrollTop = el.scrollTop;
      const initialAnchorTop = anchor && el.contains(anchor) ? anchor.getBoundingClientRect().top : null;
      const version = messageListAnchorVersionRef.current + 1;
      messageListAnchorVersionRef.current = version;
      preservingAnchorRef.current = true;
      shouldAutoScrollRef.current = false;
      cancelBrowserAnimationFrame(messageListScrollFrameRef.current);
      cancelBrowserAnimationFrame(messageListAnchorFrameRef.current);
      messageListScrollFrameRef.current = null;
      messageListAnchorFrameRef.current = null;

      const restore = () => {
        const current = messageListRef.current;
        if (!current) {
          return;
        }
        if (initialAnchorTop !== null && anchor && current.contains(anchor)) {
          current.scrollTop += anchor.getBoundingClientRect().top - initialAnchorTop;
          return;
        }
        current.scrollTop = initialScrollTop;
      };

      messageListAnchorFrameRef.current = requestBrowserAnimationFrame(() => {
        if (messageListAnchorVersionRef.current !== version) {
          return;
        }
        messageListAnchorFrameRef.current = null;
        restore();
        messageListAnchorFrameRef.current = requestBrowserAnimationFrame(() => {
          if (messageListAnchorVersionRef.current !== version) {
            return;
          }
          messageListAnchorFrameRef.current = null;
          restore();
          preservingAnchorRef.current = false;
          updateAutoScrollState();
        });
      });
    },
    [messageListRef, updateAutoScrollState],
  );

  const preserveDisclosureAnchor = useCallback(
    (event: MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Element)) return;
      const toggle = target.closest<HTMLElement>("[data-scroll-anchor-toggle]");
      if (!toggle) return;
      preserveAnchor(toggle.closest<HTMLElement>("[data-turn-id], .long-message-collapse") ?? toggle);
    },
    [preserveAnchor],
  );

  const disconnectMessageListElement = useCallback(() => {
    const observed = observedMessageListRef.current;
    if (observed) {
      observed.removeEventListener("scroll", scheduleAutoScrollStateUpdate);
      observed.removeEventListener("wheel", pauseOnUpwardWheel);
      observed.removeEventListener(MESSAGE_LIST_NAVIGATE_EVENT, handleMessageNavigation);
      observed.removeEventListener("click", preserveDisclosureAnchor, true);
    }
    messageListResizeObserverRef.current?.disconnect();
    messageListMutationObserverRef.current?.disconnect();
    cancelBrowserAnimationFrame(autoScrollStateFrameRef.current);
    cancelBrowserAnimationFrame(messageListScrollFrameRef.current);
    cancelBrowserAnimationFrame(messageListAnchorFrameRef.current);
    messageListAnchorVersionRef.current += 1;
    preservingAnchorRef.current = false;
    observedMessageListRef.current = null;
    messageListResizeObserverRef.current = null;
    messageListMutationObserverRef.current = null;
    autoScrollStateFrameRef.current = null;
    messageListScrollFrameRef.current = null;
    messageListAnchorFrameRef.current = null;
  }, [scheduleAutoScrollStateUpdate, pauseOnUpwardWheel, preserveDisclosureAnchor, handleMessageNavigation]);

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
    if (pendingInitialFollowRef.current) {
      pendingInitialFollowRef.current = false;
      shouldAutoScrollRef.current = true;
      scrollToBottomAfterLayout();
    } else {
      updateAutoScrollState();
    }
    nextElement.addEventListener("scroll", scheduleAutoScrollStateUpdate, { passive: true });
    nextElement.addEventListener("wheel", pauseOnUpwardWheel, { passive: true });
    nextElement.addEventListener(MESSAGE_LIST_NAVIGATE_EVENT, handleMessageNavigation);
    // Capture before React changes the disclosure layout, including keyboard clicks.
    nextElement.addEventListener("click", preserveDisclosureAnchor, true);

    if (typeof ResizeObserver === "function") {
      const resizeObserver = new ResizeObserver(() => {
        if (shouldAutoScrollRef.current && !preservingAnchorRef.current) {
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
        const mutationObserver = new MutationObserver((records) => {
          if (records.some((record) => record.type === "childList" && record.target === nextElement))
            observeMessageListContent();
          if (
            records.some((record) => record.type === "attributes") &&
            shouldAutoScrollRef.current &&
            !preservingAnchorRef.current
          )
            scrollToBottomAfterLayout();
        });
        mutationObserver.observe(nextElement, {
          childList: true,
          subtree: true,
          attributes: true,
          attributeFilter: ["data-preserve-scroll"],
        });
        messageListMutationObserverRef.current = mutationObserver;
      }
    }
  });

  useEffect(() => () => disconnectMessageListElement(), [disconnectMessageListElement]);

  const follow = useCallback(
    (behavior: MessageListScrollBehavior = "auto") => {
      messageListAnchorVersionRef.current += 1;
      preservingAnchorRef.current = false;
      cancelBrowserAnimationFrame(messageListAnchorFrameRef.current);
      messageListAnchorFrameRef.current = null;
      shouldAutoScrollRef.current = true;
      hasPendingNewReplyRef.current = false;
      setHasNewMessages(false);
      pendingInitialFollowRef.current = !scrollToBottomAfterLayout(behavior);
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

  useLayoutEffect(() => {
    const conversationChanged = previousConversationRef.current !== conversationId;
    const messagesChanged = previousMessagesKeyRef.current !== visibleMessagesKey;
    previousConversationRef.current = conversationId;
    previousMessagesKeyRef.current = visibleMessagesKey;

    if (conversationChanged) {
      hasPendingNewReplyRef.current = false;
      setHasNewMessages(false);
      return;
    }
    if (active && messagesChanged && !shouldAutoScrollRef.current) {
      if (!deferNewMessageNotice) {
        setHasNewMessages(true);
      } else if (latestMessageIsReply) {
        hasPendingNewReplyRef.current = true;
        if (!waitingForReply) setHasNewMessages(true);
      }
    }
  }, [active, conversationId, deferNewMessageNotice, latestMessageIsReply, visibleMessagesKey, waitingForReply]);

  useLayoutEffect(() => {
    if (deferNewMessageNotice && hasPendingNewReplyRef.current && latestMessageIsReply && !waitingForReply) {
      setHasNewMessages(true);
    }
  }, [deferNewMessageNotice, latestMessageIsReply, waitingForReply]);

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
    scrollToBottomAfterLayout("auto");
  }, [active, conversationId, messageListRef, scrollToBottomAfterLayout, visibleMessagesKey]);

  return { follow, hasNewMessages, preserveAnchor };
}
