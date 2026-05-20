import { useEffect, useRef, useState } from "react";
import { agentMatchesUser, isDirectConversation } from "@/models/conversations";
import { WorkspacePaneTypes } from "@/models/routing";

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
}) {
  const [profilePreview, setProfilePreview] = useState(null);
  const profilePreviewRef = useRef(null);

  useEffect(() => {
    if (!profilePreview) {
      return undefined;
    }

    function handlePointerDown(event) {
      const preview = profilePreviewRef.current;
      const anchor = profilePreview?.anchorEl;
      if (!preview || preview.contains(event.target) || anchor?.contains?.(event.target)) {
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

  function openParticipantPreview(user, anchor) {
    if (!user?.id) {
      return;
    }
    const rect = anchor?.getBoundingClientRect?.();
    if (!rect) {
      return;
    }
    const agent = agentItems.find((item) => agentMatchesUser(item, user));
    setProfilePreview((current) => {
      const nextType = agent ? WorkspacePaneTypes.agent : "user";
      const nextID = agent ? agent.id : user.id;
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

  function openAgentPreview(item, anchor) {
    if (!item?.id) {
      return;
    }
    const rect = anchor?.getBoundingClientRect?.();
    if (!rect) {
      return;
    }
    setProfilePreview((current) => {
      if (current?.type === WorkspacePaneTypes.agent && current?.id === item.id) {
        return null;
      }
      return {
        type: WorkspacePaneTypes.agent,
        id: item.id,
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
