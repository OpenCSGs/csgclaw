import { startTransition, useEffect, useRef, useState } from "react";
import { agentMatchesUser } from "@/models/conversations";
import { WorkspacePaneTypes } from "@/models/routing";
import type { IMUser } from "@/models/conversations";
import type { ProfilePreviewAnchorRect, ProfilePreviewController, UseProfilePreviewControllerArgs } from "./types";

type ProfilePreviewState = {
  anchorEl: HTMLElement;
  anchorRect: ProfilePreviewAnchorRect;
  id: string;
  type: "user" | typeof WorkspacePaneTypes.agent;
};

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
      setProfilePreview(null);
    }

    function handleViewportChange() {
      setProfilePreview(null);
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape") {
        return;
      }
      setProfilePreview(null);
      activeAnchor.focus();
    }

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    window.addEventListener("resize", handleViewportChange);
    window.addEventListener("scroll", handleViewportChange, true);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
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

  function openProfilePreview(user: IMUser | null | undefined, anchor: HTMLElement | null | undefined) {
    const target = profileTargetForUser(user);
    if (!target || !anchor) {
      return;
    }
    const rect = anchor.getBoundingClientRect();
    setProfilePreview((current) => {
      if (current?.type === target.type && current?.id === target.id) {
        return null;
      }
      return {
        type: target.type,
        id: target.id,
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
    openProfilePreview(user, anchor);
  }

  function closeProfilePreview() {
    setProfilePreview(null);
  }

  return {
    closeProfilePreview,
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
            onOpenAgent: (item) => {
              closeProfilePreview();
              startTransition(() => {
                selectAgent(item);
              });
            },
            onOpenDM: async (item) => {
              closeProfilePreview();
              await openAgentDirectMessage(item);
            },
          }
        : null,
  };
}
