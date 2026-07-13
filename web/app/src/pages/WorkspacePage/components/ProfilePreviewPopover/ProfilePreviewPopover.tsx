import { useLayoutEffect, useState } from "react";
import type { RefObject } from "react";
import { X } from "lucide-react";
import { ProfilePreviewContent, type ProfilePreviewAnchorRect } from "@/components/business/ProfilePreview";
import { IconButton } from "@/components/ui";
import type { AgentLike } from "@/models/agents";
import type { IMUser, TranslateFn } from "@/models/conversations";

export type ProfilePreviewPopoverProps = {
  agent: AgentLike | null;
  anchorRect: ProfilePreviewAnchorRect;
  onClose: () => void;
  onMouseEnter?: () => void;
  onMouseLeave?: () => void;
  onOpenAgent: (item: AgentLike) => void;
  onOpenDM: (item: AgentLike) => void | Promise<void>;
  previewRef: RefObject<HTMLElement | null>;
  t: TranslateFn;
  user: IMUser | null;
};

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function rootZoomScale(): number {
  const zoom = Number.parseFloat(window.getComputedStyle(document.documentElement).zoom);
  return Number.isFinite(zoom) && zoom > 0 ? zoom : 1;
}

function profilePreviewStyle(anchorRect: ProfilePreviewAnchorRect | null | undefined, cardHeight?: number | null) {
  const scale = rootZoomScale();
  const offset = 12;
  const viewportPadding = 16;
  const width = Math.min(360, (window.innerWidth - viewportPadding * 2) / scale);
  const visualWidth = width * scale;
  const maxLeft = Math.max(viewportPadding, window.innerWidth - viewportPadding - visualWidth);
  const visualCardHeight = cardHeight ?? 420 * scale;
  const visibleHeight = Math.min(visualCardHeight, window.innerHeight - viewportPadding * 2);
  const maxTop = Math.max(viewportPadding, window.innerHeight - viewportPadding - visibleHeight);

  if (!anchorRect) {
    return {
      top: `${viewportPadding / scale}px`,
      left: `${viewportPadding / scale}px`,
      width: `${width}px`,
    };
  }

  const hasRoomRight = anchorRect.right + offset + visualWidth <= window.innerWidth - viewportPadding;
  const preferredLeft = hasRoomRight ? anchorRect.right + offset : anchorRect.left - visualWidth - offset;
  const left = clamp(preferredLeft, viewportPadding, maxLeft);
  const top = clamp(anchorRect.top - 12, viewportPadding, maxTop);
  return { top: `${top / scale}px`, left: `${left / scale}px`, width: `${width}px` };
}

export function ProfilePreviewPopover({
  previewRef,
  agent,
  user,
  anchorRect,
  t,
  onClose,
  onMouseEnter,
  onMouseLeave,
  onOpenAgent,
  onOpenDM,
}: ProfilePreviewPopoverProps) {
  const [cardHeight, setCardHeight] = useState<number | null>(null);

  useLayoutEffect(() => {
    const preview = previewRef?.current;
    if (!preview) {
      return;
    }
    const nextHeight = Math.ceil(preview.getBoundingClientRect().height);
    if (nextHeight > 0 && nextHeight !== cardHeight) {
      setCardHeight(nextHeight);
    }
  }, [previewRef, cardHeight, agent?.id, user?.id]);

  return (
    <aside
      ref={previewRef}
      className="profile-preview-popover"
      style={profilePreviewStyle(anchorRect, cardHeight)}
      aria-label={t("profilePreview")}
      role="dialog"
      aria-modal="false"
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <div className="preview-header">
        <div className="preview-title">{t("profilePreview")}</div>
        <IconButton
          className="modal-close"
          icon={<X size={20} strokeWidth={2} />}
          label={t("close")}
          markClassName="modal-close-icon"
          onClick={onClose}
          variant="tertiaryGray"
        />
      </div>
      <ProfilePreviewContent agent={agent} user={user} t={t} onOpenAgent={onOpenAgent} onOpenDM={onOpenDM} />
    </aside>
  );
}
