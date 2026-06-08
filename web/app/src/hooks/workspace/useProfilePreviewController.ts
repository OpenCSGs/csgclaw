import { useEffect, useRef, useState } from "react";
import { agentMatchesUser, isDirectConversation } from "@/models/conversations";
import { WorkspacePaneTypes } from "@/models/routing";
import type { AgentLike } from "@/models/agents";
import type { IMUser } from "@/models/conversations";
import type { ProfilePreviewAnchorRect, ProfilePreviewController, UseProfilePreviewControllerArgs } from "./types";

type ProfilePreviewState = {
  anchorEl: HTMLElement;
  anchorRect: ProfilePreviewAnchorRect;
  id: string;
  type: "user" | typeof WorkspacePaneTypes.agent;
};

export function useProfilePreviewController({
  agentActionBusy,
  agentItems,
  closeConversationTools,
  deletePreviewBot,
  openAgentDirectMessage,
  selectedConversation,
  selectAgent,
  t,
  usersById,
}: UseProfilePreviewControllerArgs): ProfilePreviewController {
  const [profilePreview, setProfilePreview] = useState<ProfilePreviewState | null>(null);
  const profilePreviewRef = useRef<HTMLElement | null>(null);

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
      closeProfilePreview();
    }

    function handleViewportChange() {
      closeProfilePreview();
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

  const previewUser =
    profilePreview?.type === "user"
      ? (usersById.get(profilePreview.id) ?? null)
      : profilePreview?.type === WorkspacePaneTypes.agent
        ? (usersById.get(profilePreview.id) ?? null)
        : null;
  const previewAgent = profilePreview
    ? (agentItems.find((item) => item.id === profilePreview.id || agentMatchesUser(item, previewUser)) ?? null)
    : null;

  function openParticipantPreview(user: IMUser | null | undefined, anchor: HTMLElement | null | undefined) {
    if (!user?.id || !anchor) {
      return;
    }
    const rect = anchor.getBoundingClientRect();
    const agent = agentItems.find((item) => agentMatchesUser(item, user));
    const nextID = String(agent?.id || user.id).trim();
    if (!nextID) {
      return;
    }
    setProfilePreview((current) => {
      const nextType = agent ? WorkspacePaneTypes.agent : "user";
      if (current?.type === nextType && current?.id === nextID) {
        return null;
      }
      return {
        type: nextType,
        id: nextID,
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

  function openAgentPreview(item: AgentLike | null | undefined, anchor: HTMLElement | null | undefined) {
    if (!item?.id || !anchor) {
      return;
    }
    const itemID = item.id;
    const rect = anchor.getBoundingClientRect();
    setProfilePreview((current) => {
      if (current?.type === WorkspacePaneTypes.agent && current?.id === itemID) {
        return null;
      }
      return {
        type: WorkspacePaneTypes.agent,
        id: itemID,
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

  function closeProfilePreview() {
    setProfilePreview(null);
  }

  return {
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
            inDirectConversation: Boolean(selectedConversation && isDirectConversation(selectedConversation)),
            busyKey: agentActionBusy,
            onClose: closeProfilePreview,
            onOpenAgent: (item) => {
              selectAgent(item);
              closeProfilePreview();
            },
            onOpenDM: async (item) => {
              await openAgentDirectMessage(item);
              closeProfilePreview();
            },
            onDelete: async (item) => {
              const deleted = await deletePreviewBot(item);
              if (deleted) {
                closeProfilePreview();
              }
            },
          }
        : null,
  };
}
