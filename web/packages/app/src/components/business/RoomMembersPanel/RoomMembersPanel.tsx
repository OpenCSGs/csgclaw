import { useEffect, useId, useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { BellRing, Trash2, X } from "lucide-react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { Button, Checkbox, IconButton, Switch } from "@/components/ui";
import { isNotificationBotAgent } from "@/models/agents";
import type { AgentLike } from "@/models/agents";
import { resolveAgentForUser } from "@/models/conversations";
import type { IMUser, TranslateFn } from "@/models/conversations";
import { avatarFallbackText } from "@/shared/avatar";
import { classNames } from "@/shared/lib/classNames";
import { toggleSelection } from "@/shared/lib/collections";
import styles from "./RoomMembersPanel.module.css";

export type RoomMembersPanelProps = {
  agents?: AgentLike[];
  candidates: IMUser[];
  currentUserID: string;
  inviteUserIDs: string[];
  inviteBusy?: boolean;
  members: IMUser[];
  memberActionBusyID?: string;
  memberActionError?: string;
  onRemoveMember: (memberID: string) => void | Promise<void>;
  onExternalMembershipChange: (memberID: string, connected: boolean) => void | Promise<void>;
  onClose: () => void;
  onInvite: () => void | Promise<void>;
  onInviteUserIDsChange: Dispatch<SetStateAction<string[]>>;
  onManageExternalMessages?: () => void;
  onPreviewUser?: (user: IMUser, anchor: HTMLElement) => void;
  submitError?: string;
  t: TranslateFn;
};

export function RoomMembersPanel({
  agents = [],
  candidates,
  currentUserID,
  inviteUserIDs,
  inviteBusy = false,
  members,
  memberActionBusyID = "",
  memberActionError = "",
  onRemoveMember,
  onExternalMembershipChange,
  onClose,
  onInvite,
  onInviteUserIDsChange,
  onManageExternalMessages,
  onPreviewUser,
  submitError = "",
  t,
}: RoomMembersPanelProps) {
  const titleID = useId();
  const panelRef = useRef<HTMLElement>(null);
  const [overlay, setOverlay] = useState(false);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const isExternal = (user: IMUser) =>
    user.participants?.some((participant) => participant.type === "notification") ||
    isNotificationBotAgent(resolveAgentForUser(agents, user));
  const regularMembers = members.filter((user) => !isExternal(user));
  const regularCandidates = candidates.filter((user) => !isExternal(user));
  const externalMembers = members.filter(isExternal);
  const externalCandidates = candidates.filter(isExternal);
  const candidateIDs = regularCandidates.map((user) => user.id);
  const allSelected = candidateIDs.length > 0 && candidateIDs.every((id) => inviteUserIDs.includes(id));
  const selectedCount = candidateIDs.filter((id) => inviteUserIDs.includes(id)).length;
  const busy = inviteBusy || Boolean(memberActionBusyID);

  useEffect(() => {
    const parent = panelRef.current?.parentElement;
    if (!parent) return;
    const measure = () => setOverlay(parent.getBoundingClientRect().width < 780);
    measure();
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(measure);
    observer?.observe(parent);
    window.addEventListener("resize", measure);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);

  useEffect(() => {
    if (!overlay) return;
    const panel = panelRef.current;
    const siblings = Array.from(panel?.parentElement?.children ?? []).filter(
      (node): node is HTMLElement => node instanceof HTMLElement && node !== panel,
    );
    const previous = siblings.map((node) => Boolean(node.inert));
    siblings.forEach((node) => (node.inert = true));
    return () => siblings.forEach((node, index) => (node.inert = previous[index]));
  }, [overlay]);

  useEffect(() => {
    const panel = panelRef.current;
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeButtonRef.current?.focus({ preventScroll: true });
    const handleEscape = (event: KeyboardEvent) => {
      if (
        event.key === "Escape" &&
        !event.defaultPrevented &&
        !document.querySelector('[role="dialog"], [data-state="open"][role="menu"]') &&
        panelRef.current?.contains(event.target as Node)
      ) {
        event.preventDefault();
        onCloseRef.current();
      }
    };
    window.addEventListener("keydown", handleEscape);
    return () => {
      window.removeEventListener("keydown", handleEscape);
      if (
        previous?.isConnected &&
        (document.activeElement === document.body || panel?.contains(document.activeElement))
      ) {
        queueMicrotask(() => previous.isConnected && previous.focus({ preventScroll: true }));
      }
    };
  }, []);

  function renderMember(user: IMUser) {
    return (
      <div key={user.id} className={styles.memberRow}>
        {onPreviewUser ? (
          <button
            type="button"
            className={styles.avatarButton}
            aria-label={`${t("profilePreview")} ${user.name || user.id}`}
            onClick={(event) => onPreviewUser(user, event.currentTarget)}
          >
            <MemberAvatar user={user} />
          </button>
        ) : (
          <MemberAvatar user={user} />
        )}
        <MemberCopy user={user} />
        {user.id === currentUserID ? (
          <span className={styles.status}>{t("memberOwner")}</span>
        ) : (
          <Button
            iconOnly
            size="sm"
            variant="tertiaryGray"
            aria-label={`${t("removeMember")} ${user.name || user.id}`}
            title={`${t("removeMember")} ${user.name || user.id}`}
            disabled={busy}
            loading={memberActionBusyID === user.id}
            loadingLabel={t("removing")}
            onClick={() => void onRemoveMember(user.id)}
          >
            <Trash2 size={15} aria-hidden="true" />
          </Button>
        )}
      </div>
    );
  }

  function renderCandidate(user: IMUser) {
    return (
      <label key={user.id} className={classNames(styles.memberRow, styles.candidateRow)}>
        <Checkbox
          checked={inviteUserIDs.includes(user.id)}
          disabled={busy}
          aria-label={user.name || user.id}
          onCheckedChange={() => onInviteUserIDsChange((current) => toggleSelection(current, user.id))}
        />
        <MemberAvatar user={user} />
        <MemberCopy user={user} />
      </label>
    );
  }

  return (
    <aside ref={panelRef} className={styles.panel} data-overlay={overlay || undefined} aria-labelledby={titleID}>
      <header className={styles.header}>
        <div>
          <h2 id={titleID}>{t("memberManagementTitle")}</h2>
          <p>{t("memberManagementSubtitle")}</p>
        </div>
        <IconButton
          ref={closeButtonRef}
          icon={<X size={18} />}
          label={t("close")}
          variant="tertiaryGray"
          size="sm"
          onClick={onClose}
        />
      </header>
      <div className={styles.content}>
        <section className={styles.section} aria-label={t("currentMembers")}>
          <h3>{t("currentMembers")}</h3>
          <div className={styles.memberList}>{regularMembers.map(renderMember)}</div>
        </section>
        <section className={styles.section} aria-label={t("addMembers")}>
          <h3>{t("addMembers")}</h3>
          <div className={styles.memberList}>
            {regularCandidates.length ? (
              <>
                <label className={classNames(styles.memberRow, styles.selectAll)}>
                  <Checkbox
                    checked={allSelected ? true : selectedCount > 0 ? "indeterminate" : false}
                    disabled={busy}
                    aria-label={t("allMembers")}
                    onCheckedChange={() =>
                      onInviteUserIDsChange((current) =>
                        candidateIDs.every((id) => current.includes(id))
                          ? current.filter((id) => !candidateIDs.includes(id))
                          : Array.from(new Set([...current, ...candidateIDs])),
                      )
                    }
                  />
                  <span className={styles.memberCopy}>
                    <strong>{t("allMembers")}</strong>
                    <small>
                      {selectedCount}/{candidateIDs.length}
                    </small>
                  </span>
                  <span className={styles.avatarGroup} aria-hidden="true">
                    {regularCandidates.slice(0, 6).map((user) => (
                      <MemberAvatar key={user.id} user={user} />
                    ))}
                    {regularCandidates.length > 6 ? (
                      <span className={styles.avatarMore}>+{regularCandidates.length - 6}</span>
                    ) : null}
                  </span>
                </label>
                {regularCandidates.map(renderCandidate)}
              </>
            ) : (
              <p className={styles.hint}>{t("noInviteCandidates")}</p>
            )}
          </div>
        </section>
        <section className={styles.section} aria-label={t("externalMessagesTitle")}>
          <h3>{t("externalMessagesTitle")}</h3>
          <p className={styles.hint}>{t("externalMessagesHint")}</p>
          <div className={styles.memberList}>
            {[...externalMembers, ...externalCandidates].map((user) => {
              const connected = externalMembers.some((member) => member.id === user.id);
              const status = connected ? t("externalMessagesConnected") : t("externalMessagesDisconnected");
              return (
                <div key={user.id} className={styles.memberRow}>
                  <span className={styles.avatar}>
                    <BellRing size={17} aria-hidden="true" />
                  </span>
                  <span className={styles.memberCopy}>
                    <strong>{user.name || user.id}</strong>
                    <small>{status}</small>
                  </span>
                  <Switch
                    checked={connected}
                    disabled={busy}
                    aria-label={`${user.name || user.id} · ${status}`}
                    onCheckedChange={(next) => void onExternalMembershipChange(user.id, next)}
                  />
                </div>
              );
            })}
            {!externalMembers.length && !externalCandidates.length ? (
              <p className={styles.hint}>{t("externalMessagesEmpty")}</p>
            ) : null}
          </div>
          {onManageExternalMessages ? (
            <Button variant="linkColor" size="sm" onClick={onManageExternalMessages}>
              {t("externalMessagesManage")}
            </Button>
          ) : null}
        </section>
        {memberActionError || submitError ? (
          <p className={styles.error} role="alert">
            {memberActionError || submitError}
          </p>
        ) : null}
      </div>
      <footer className={styles.footer}>
        <Button variant="secondaryGray" size="md" onClick={onClose}>
          {t("cancel")}
        </Button>
        <Button
          variant="primary"
          size="md"
          disabled={busy || !inviteUserIDs.length}
          loading={inviteBusy}
          onClick={() => void onInvite()}
        >
          {t("inviteSelectedMembers")}
        </Button>
      </footer>
    </aside>
  );
}

function MemberAvatar({ user }: { user: IMUser }) {
  return (
    <span className={styles.avatar}>
      <AgentAvatarContent avatar={user.avatar} fallback={avatarFallbackText(user.avatar, user.name, user.id)} />
    </span>
  );
}

function MemberCopy({ user }: { user: IMUser }) {
  return (
    <span className={styles.memberCopy}>
      <strong>{user.name || user.id}</strong>
      <small>{user.id}</small>
    </span>
  );
}
