import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { avatarFallbackText } from "@/shared/avatar";
import type { IMConversation, IMUser, UsersById } from "@/models/conversations";
import "./RoomAvatar.css";

type RoomAvatarMember = Pick<IMUser, "accent_hex" | "avatar" | "handle" | "id" | "name">;
type RoomAvatarSlot = RoomAvatarMember & { placeholder?: boolean };

type RoomAvatarProps = {
  ariaLabel?: string;
  count?: number;
  members: RoomAvatarMember[];
  size?: 32 | 48;
  showCountBadge?: boolean;
};

const TILE_COLORS = ["#c4a4f3", "#fd94a1", "#99c6f8", "#fecd79", "#b68de3"] as const;

function normalizeMemberFallback(member: RoomAvatarSlot): string {
  if (member.placeholder) {
    return "#";
  }
  return avatarFallbackText(member.name, member.handle, member.id);
}

function pickTileColor(member: RoomAvatarMember, index: number): string {
  return member.accent_hex || TILE_COLORS[index % TILE_COLORS.length];
}

function buildAvatarTiles(memberCount: number): Array<{ sizeClass: string; variant: string }> {
  if (memberCount <= 1) {
    return [{ sizeClass: "room-avatar-tile--full", variant: "single" }];
  }
  if (memberCount === 2) {
    return [
      { sizeClass: "room-avatar-tile--half room-avatar-tile--left", variant: "top-left" },
      { sizeClass: "room-avatar-tile--half room-avatar-tile--right", variant: "top-right" },
    ];
  }
  if (memberCount === 3) {
    return [
      { sizeClass: "room-avatar-tile--quarter room-avatar-tile--top-left", variant: "top-left" },
      { sizeClass: "room-avatar-tile--quarter room-avatar-tile--bottom-left", variant: "bottom-left" },
      { sizeClass: "room-avatar-tile--tall room-avatar-tile--right", variant: "right" },
    ];
  }
  return [
    { sizeClass: "room-avatar-tile--quarter room-avatar-tile--top-left", variant: "top-left" },
    { sizeClass: "room-avatar-tile--quarter room-avatar-tile--top-right", variant: "top-right" },
    { sizeClass: "room-avatar-tile--quarter room-avatar-tile--bottom-left", variant: "bottom-left" },
    { sizeClass: "room-avatar-tile--quarter room-avatar-tile--bottom-right", variant: "bottom-right" },
  ];
}

export function resolveRoomAvatarMembers(
  conversation: IMConversation | null | undefined,
  usersById: UsersById,
  currentUserID?: string | null,
): RoomAvatarMember[] {
  return (conversation?.members || [])
    .filter((memberID) => memberID !== currentUserID)
    .map((memberID) => usersById.get(memberID))
    .filter((member): member is IMUser => Boolean(member))
    .map((member) => ({
      id: member.id,
      name: member.name || member.handle || member.id,
      handle: member.handle,
      avatar: member.avatar,
      accent_hex: member.accent_hex,
    }));
}

export function RoomAvatar({ ariaLabel, count, members, size = 32, showCountBadge = true }: RoomAvatarProps) {
  const totalCount = count ?? members.length;
  const visibleMembers = members.slice(0, 4);
  const variantCount = Math.min(4, Math.max(1, visibleMembers.length));
  const tiles = buildAvatarTiles(variantCount);
  const shouldShowBadge = showCountBadge && totalCount > 4;
  const isCompact = size <= 32;

  return (
    <span
      aria-hidden={!ariaLabel}
      aria-label={ariaLabel || undefined}
      className={`room-avatar ${isCompact ? "room-avatar--compact" : "room-avatar--large"} room-avatar--count-${variantCount}`.trim()}
      style={{
        borderRadius: Math.round(size / 2),
        height: size,
        width: size,
      }}
    >
      {tiles.map((tile, index) => {
        const member = visibleMembers[index];
        if (!member) {
          return null;
        }
        return (
          <span
            aria-hidden="true"
            className={`room-avatar-tile ${tile.sizeClass}`}
            key={`${member.id}-${tile.variant}`}
            style={{ backgroundColor: pickTileColor(member, index) }}
          >
            <span className="room-avatar-content">
              <AgentAvatarContent avatar={member.avatar} fallback={normalizeMemberFallback(member)} />
            </span>
          </span>
        );
      })}
      {shouldShowBadge ? (
        <span aria-hidden="true" className="room-avatar-count-badge">
          {totalCount}
        </span>
      ) : null}
    </span>
  );
}
