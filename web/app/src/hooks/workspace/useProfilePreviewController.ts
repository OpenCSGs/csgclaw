import { useEffect, useRef, useState } from "react";
import { agentMatchesUser } from "@/models/conversations";
import { WorkspacePaneTypes } from "@/models/routing";
import type { AgentLike } from "@/models/agents";
import type { IMUser } from "@/models/conversations";
import type { ProfilePreviewAnchorRect, ProfilePreviewController, UseProfilePreviewControllerArgs } from "./types";

type ProfilePreviewState = {
  anchorEl: HTMLElement;
  anchorRect: ProfilePreviewAnchorRect;
  id: string;
  mode: "hover" | "manual";
  type: "user" | typeof WorkspacePaneTypes.agent;
};

const PROFILE_PREVIEW_CLOSE_DELAY_MS = 120;

export function useProfilePreviewController({
  agentItems,
  closeConversationTools,
  openAgentDirectMessage,
  selectAgent,
  t,
  usersById,
}: UseProfilePreviewControllerArgs): ProfilePreviewController {
  const [profilePreview, setProfilePreview] = useState<ProfilePreviewState | null>(null);
  const profilePreviewRef = useRef<HTMLElement | null>(null);
  const profilePreviewCloseTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  function clearProfilePreviewCloseTimer() {
    const timer = profilePreviewCloseTimerRef.current;
    if (!timer) {
      return;
    }
    clearTimeout(timer);
    profilePreviewCloseTimerRef.current = null;
  }

  useEffect(() => {
    const activePreview = profilePreview;
    if (!activePreview) {
      return undefined;
    }
    const activeAnchor = activePreview.anchorEl;

    function handlePointerDown(event: MouseEvent) {
      const preview = profilePreviewRef.current;
      if (
        !preview ||
        !(event.target instanceof Node) ||
        preview.contains(event.target) ||
        activeAnchor.contains(event.target)
      ) {
        return;
      }
      clearProfilePreviewCloseTimer();
      setProfilePreview(null);
    }

    function handleViewportChange() {
      clearProfilePreviewCloseTimer();
      setProfilePreview(null);
    }

    document.addEventListener("mousedown", handlePointerDown);
    window.addEventListener("resize", handleViewportChange);
    window.addEventListener("scroll", handleViewportChange, true);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      window.removeEventListener("resize", handleViewportChange);
      window.removeEventListener("scroll", handleViewportChange, true);
    };
  }, [profilePreview]);

  useEffect(() => {
    return () => clearProfilePreviewCloseTimer();
  }, []);

  const previewUser =
    profilePreview?.type === "user"
      ? (usersById.get(profilePreview.id) ?? null)
      : profilePreview?.type === WorkspacePaneTypes.agent
        ? (usersById.get(profilePreview.id) ?? null)
        : null;
  const previewAgent = profilePreview
    ? (agentItems.find((item) => item.id === profilePreview.id || agentMatchesUser(item, previewUser)) ?? null)
    : null;

  function profileTargetForUser(user: IMUser | null | undefined) {
    if (!user?.id) {
      return null;
    }
    const agent = agentItems.find((item) => agentMatchesUser(item, user));
    const id = String(agent?.id || user.id).trim();
    if (!id) {
      return null;
    }
    return {
      id,
      type: agent ? WorkspacePaneTypes.agent : ("user" as const),
    };
  }

  function openProfilePreview(
    user: IMUser | null | undefined,
    anchor: HTMLElement | null | undefined,
    mode: ProfilePreviewState["mode"],
  ) {
    const target = profileTargetForUser(user);
    if (!target || !anchor) {
      return;
    }
    clearProfilePreviewCloseTimer();
    const rect = anchor.getBoundingClientRect();
    setProfilePreview((current) => {
      if (
        mode === "manual" &&
        current?.mode === "manual" &&
        current?.type === target.type &&
        current?.id === target.id
      ) {
        return null;
      }
      return {
        type: target.type,
        id: target.id,
        mode,
        anchorRect: {
          top: rect.top,
          right: rect.right,
          bottom: rect.bottom,
          left: rect.left,
        },
        anchorEl: anchor,
      };
    });
    closeConversationTools();
  }

  function openAgentProfilePreview(
    item: AgentLike | null | undefined,
    anchor: HTMLElement | null | undefined,
    mode: ProfilePreviewState["mode"],
  ) {
    if (!item?.id || !anchor) {
      return;
    }
    clearProfilePreviewCloseTimer();
    const itemID = item.id;
    const rect = anchor.getBoundingClientRect();
    setProfilePreview((current) => {
      if (
        mode === "manual" &&
        current?.mode === "manual" &&
        current?.type === WorkspacePaneTypes.agent &&
        current?.id === itemID
      ) {
        return null;
      }
      return {
        type: WorkspacePaneTypes.agent,
        id: itemID,
        mode,
        anchorRect: {
          top: rect.top,
          right: rect.right,
          bottom: rect.bottom,
          left: rect.left,
        },
        anchorEl: anchor,
      };
    });
    closeConversationTools();
  }

  function openParticipantPreview(user: IMUser | null | undefined, anchor: HTMLElement | null | undefined) {
    openProfilePreview(user, anchor, "manual");
  }

  function showParticipantPreview(user: IMUser | null | undefined, anchor: HTMLElement | null | undefined) {
    openProfilePreview(user, anchor, "hover");
  }

  function openAgentPreview(item: AgentLike | null | undefined, anchor: HTMLElement | null | undefined) {
    openAgentProfilePreview(item, anchor, "manual");
  }

  function showAgentPreview(item: AgentLike | null | undefined, anchor: HTMLElement | null | undefined) {
    openAgentProfilePreview(item, anchor, "hover");
  }

  function closeProfilePreview() {
    clearProfilePreviewCloseTimer();
    setProfilePreview(null);
  }

  function scheduleProfilePreviewClose() {
    clearProfilePreviewCloseTimer();
    profilePreviewCloseTimerRef.current = setTimeout(() => {
      profilePreviewCloseTimerRef.current = null;
      setProfilePreview((current) => (current?.mode === "hover" ? null : current));
    }, PROFILE_PREVIEW_CLOSE_DELAY_MS);
  }

  function cancelProfilePreviewClose() {
    clearProfilePreviewCloseTimer();
  }

  return {
    cancelProfilePreviewClose,
    closeProfilePreview,
    openAgentPreview,
    openParticipantPreview,
    profilePreviewProps:
      profilePreview && (previewAgent || previewUser)
        ? {
            previewRef: profilePreviewRef,
            agent: previewAgent,
            user: previewUser,
            anchorRect: profilePreview.anchorRect,
            t,
            onClose: closeProfilePreview,
            onMouseEnter: cancelProfilePreviewClose,
            onMouseLeave: scheduleProfilePreviewClose,
            onOpenAgent: (item) => {
              selectAgent(item);
              closeProfilePreview();
            },
            onOpenDM: async (item) => {
              await openAgentDirectMessage(item);
              closeProfilePreview();
            },
          }
        : null,
    scheduleProfilePreviewClose,
    showAgentPreview,
    showParticipantPreview,
  };
}
