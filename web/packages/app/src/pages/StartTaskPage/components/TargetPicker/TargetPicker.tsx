import type { CloudAgent } from "@/models/cloudAgents";
import { useMemo, useRef, useState, type ReactNode } from "react";
import { Check, ChevronDown, Plus, Search } from "lucide-react";
import { Button, TextInput, PopoverContent, PopoverRoot, PopoverTrigger } from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { EntityPinHeading, EntityPinRow } from "@/components/business/EntityPinRow";
import { entityPinKey, partitionPinned, usePinnedEntities } from "@/hooks/workspace/usePinnedEntities";
import { RoomAvatar } from "@/components/business/RoomAvatar";
import { agentRoleLabel, isManagerAgent as isManagerAgentItem } from "@/models/agents";
import { resolveTeamAvatarMembers } from "@/models/tasks";
import { classNames } from "@/shared/lib/classNames";
import styles from "./TargetPicker.module.css";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn, UsersById } from "@/models/conversations";
import type { WorkspaceTeam } from "@/models/tasks";

export type TargetKind = "agent" | "team" | "cloud";

export const SOLO_TEAM_ID = "__solo__";

export type Target = {
  kind: TargetKind;
  id: string;
};

export type TargetPickerProps = {
  agents: readonly AgentLike[];
  cloudAgents?: readonly CloudAgent[];
  teams: readonly WorkspaceTeam[];
  currentUserID?: string;
  usersById?: UsersById | null;
  value: Target;

  locked?: boolean;
  t: TranslateFn;
  onChange: (next: Target) => void;

  onCreateAgent: () => void;
  onCreateTeam: () => void;
};

export function TargetPicker({
  agents,
  cloudAgents = [],
  teams,
  usersById,
  currentUserID = "",
  value,
  locked = false,
  t,
  onChange,
  onCreateAgent,
  onCreateTeam,
}: TargetPickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [pinnedExpanded, setPinnedExpanded] = useState(true);
  const pins = usePinnedEntities(currentUserID);
  const searchRef = useRef<HTMLInputElement | null>(null);

  const managerAgent = useMemo(() => agents.find((item) => isManagerAgentItem(item)) ?? agents[0] ?? null, [agents]);

  const isSolo = value.kind === "team" && value.id === SOLO_TEAM_ID;
  const selectedAgent = value.kind === "agent" ? (agents.find((item) => item.id === value.id) ?? null) : null;
  const selectedCloud = value.kind === "cloud" ? cloudAgents.find((item) => item.id === value.id) : null;
  const selectedTeam = value.kind === "team" && !isSolo ? (teams.find((item) => item.id === value.id) ?? null) : null;

  const fallbackAgent = agents[0] ?? null;
  const triggerName = isSolo
    ? t("iaStartRoomSolo")
    : selectedCloud?.name || selectedAgent?.name || selectedTeam?.title || fallbackAgent?.name || "";

  const needle = query.trim().toLocaleLowerCase();
  const visibleAgents = useMemo(() => {
    if (!needle) {
      return agents;
    }
    return agents.filter((item) =>
      String(item.name ?? "")
        .toLocaleLowerCase()
        .includes(needle),
    );
  }, [agents, needle]);
  const visibleCloud = cloudAgents.filter((item) => !needle || item.name.toLocaleLowerCase().includes(needle));
  const visibleTeams = useMemo(() => {
    if (!needle) {
      return teams;
    }
    return teams.filter((team) =>
      String(team.title ?? "")
        .toLocaleLowerCase()
        .includes(needle),
    );
  }, [teams, needle]);

  function close() {
    setOpen(false);
    setQuery("");
  }

  function pick(next: Target) {
    onChange(next);
    close();
  }

  type Row = Target & { name: string; content: ReactNode };
  const rows: Row[] = [
    ...visibleAgents.flatMap((item): Row[] => {
      const id = item.id || "";
      if (!id) return [];
      const active = value.kind === "agent" && value.id === id;
      return [
        {
          kind: "agent",
          id,
          name: item.name || id,
          content: (
            <button
              type="button"
              aria-current={active ? "true" : undefined}
              className={styles.row}
              onClick={() => pick({ kind: "agent", id })}
            >
              {active ? <Check aria-hidden="true" className={styles.rowCheck} size={15} strokeWidth={2.4} /> : null}
              <span className={classNames("iaAvatarFrame", styles.rowIcon)} aria-hidden="true">
                <AgentAvatarContent avatar={item.avatar} fallback={String(item.name || id).slice(0, 1)} />
              </span>
              <span className={styles.rowText}>
                <span className={styles.rowName}>{item.name || id}</span>
              </span>
              {}
              <span className={styles.rowBadge}>{agentRoleLabel(item, t)}</span>
            </button>
          ),
        },
      ];
    }),
    ...visibleCloud.map(
      (item): Row => ({
        kind: "cloud",
        id: item.id,
        name: item.name,
        content: (
          <button
            type="button"
            disabled={item.status !== "online"}
            title={item.status !== "online" ? t("cloudChatOffline") : item.description}
            aria-current={value.kind === "cloud" && value.id === item.id ? "true" : undefined}
            className={styles.row}
            onClick={() => pick({ kind: "cloud", id: item.id })}
          >
            {value.kind === "cloud" && value.id === item.id ? (
              <Check aria-hidden="true" className={styles.rowCheck} size={15} />
            ) : null}
            <span className={classNames("iaAvatarFrame", styles.rowIcon)} aria-hidden="true">
              <AgentAvatarContent avatar={item.avatar} fallback={item.name.slice(0, 1)} />
            </span>
            <span className={styles.rowText}>
              <span className={styles.rowName}>{item.name}</span>
            </span>
            <span className={styles.rowBadge}>
              {t("agentPlazaCloud")} · {t(item.status === "online" ? "agentPlazaOnline" : "agentPlazaOffline")}
            </span>
          </button>
        ),
      }),
    ),
    ...visibleTeams.map((team): Row => {
      const active = value.kind === "team" && value.id === team.id;
      return {
        kind: "team",
        id: team.id,
        name: team.title,
        content: (
          <button
            type="button"
            aria-current={active ? "true" : undefined}
            className={styles.row}
            onClick={() => pick({ kind: "team", id: team.id })}
          >
            {active ? <Check aria-hidden="true" className={styles.rowCheck} size={15} strokeWidth={2.4} /> : null}
            <span className={classNames("iaAvatarFrame", styles.rowIcon)} aria-hidden="true">
              <RoomAvatar
                count={team.member_agent_ids.length}
                members={resolveTeamAvatarMembers(team, agents, usersById)}
                showCountBadge={false}
                size={22}
              />
            </span>
            <span className={styles.rowText}>
              <span className={styles.rowName}>{team.title}</span>
            </span>
            <span className={styles.rowBadge}>{t("iaTargetTeamMembers", { count: team.member_agent_ids.length })}</span>
          </button>
        ),
      };
    }),
  ];
  const { pinned, rest } = partitionPinned(rows, pins.order);
  const groups = [
    { key: "pinned", label: t("iaPinned"), rows: pinned, solo: false },
    { key: "agent", label: t("iaTargetGroupAgents"), rows: rest.filter((row) => row.kind === "agent"), solo: false },
    { key: "cloud", label: t("agentPlazaCloud"), rows: rest.filter((row) => row.kind === "cloud"), solo: false },
    { key: "team", label: t("iaTargetGroupTeams"), rows: rest.filter((row) => row.kind === "team"), solo: !needle },
  ];

  return (
    <PopoverRoot
      open={open}
      onOpenChange={(next) => {
        if (locked && next) {
          return;
        }
        setOpen(next);
        if (!next) {
          setQuery("");
        }
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={`${t("iaTargetLabel")}：${triggerName}`}
          className={styles.trigger}
          disabled={locked}
          title={locked ? t("iaTargetLocked") : t("iaTargetChange")}
        >
          <span className={classNames("iaAvatarFrame", styles.triggerAvatar)} aria-hidden="true">
            {isSolo ? (
              <AgentAvatarContent avatar={managerAgent?.avatar} fallback={triggerName.slice(0, 1)} />
            ) : selectedTeam ? (
              <RoomAvatar
                count={selectedTeam.member_agent_ids.length}
                members={resolveTeamAvatarMembers(selectedTeam, agents, usersById)}
                showCountBadge={false}
                size={22}
              />
            ) : (
              <AgentAvatarContent
                avatar={selectedCloud?.avatar ?? selectedAgent?.avatar ?? fallbackAgent?.avatar}
                fallback={triggerName.slice(0, 1)}
              />
            )}
          </span>
          <span className={styles.triggerLabel}>
            {}
            {isSolo
              ? triggerName
              : selectedTeam
                ? t("iaTargetTeamChip", { name: triggerName })
                : t("iaTargetAgentChip", { name: triggerName })}
          </span>
          {selectedCloud ? <span className={styles.rowBadge}>{t("agentPlazaCloud")}</span> : null}
          <ChevronDown aria-hidden="true" className={styles.triggerChevron} size={14} strokeWidth={2.2} />
        </button>
      </PopoverTrigger>

      <PopoverContent
        align="start"
        className={styles.popover}
        side="bottom"
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          searchRef.current?.focus();
        }}
      >
        <div className={styles.searchField}>
          <Search aria-hidden="true" size={14} strokeWidth={2} />
          <TextInput
            variant="plain"
            ref={searchRef}
            aria-label={t("iaTargetSearch")}
            className={styles.searchInput}
            placeholder={t("iaTargetSearch")}
            type="text"
            value={query}
            onChange={(event) => {
              const nextQuery = event.currentTarget.value;
              if (nextQuery.trim()) setPinnedExpanded(true);
              setQuery(nextQuery);
            }}
          />
        </div>

        <div className={styles.list}>
          {visibleAgents.length === 0 && visibleTeams.length === 0 && visibleCloud.length === 0 ? (
            <p className={styles.empty}>{t("iaTargetEmpty")}</p>
          ) : null}

          <ul className={styles.items}>
            {groups.flatMap((group) => {
              const isPinned = group.key === "pinned";
              const visibleRows = isPinned && !pinnedExpanded ? [] : group.rows;
              return group.rows.length || group.solo
                ? [
                    <li key={group.key} className={isPinned ? undefined : styles.groupLabel}>
                      {isPinned ? (
                        <EntityPinHeading
                          expanded={pinnedExpanded}
                          onToggle={() => setPinnedExpanded((expanded) => !expanded)}
                          t={t}
                        />
                      ) : (
                        group.label
                      )}
                    </li>,
                    ...(group.solo
                      ? [
                          <li key={SOLO_TEAM_ID}>
                            <button
                              type="button"
                              className={classNames(
                                styles.row,
                                styles.soloRow,
                                value.kind === "team" && value.id === SOLO_TEAM_ID && styles.rowActive,
                              )}
                              onClick={() => pick({ kind: "team", id: SOLO_TEAM_ID })}
                            >
                              {value.kind === "team" && value.id === SOLO_TEAM_ID ? (
                                <Check aria-hidden="true" className={styles.rowCheck} size={15} strokeWidth={2.4} />
                              ) : null}
                              <span className={classNames("iaAvatarFrame", styles.rowIcon)} aria-hidden="true">
                                <AgentAvatarContent
                                  avatar={managerAgent?.avatar}
                                  fallback={String(managerAgent?.name || "M").slice(0, 1)}
                                />
                              </span>
                              <span className={styles.rowText}>
                                <span className={styles.rowName}>{t("iaStartRoomSolo")}</span>
                              </span>
                            </button>
                          </li>,
                        ]
                      : []),
                    ...visibleRows.map((row) => (
                      <EntityPinRow
                        as="li"
                        key={entityPinKey(row)}
                        name={row.name}
                        pinned={isPinned}
                        t={t}
                        onToggle={() => {
                          if (!isPinned) setPinnedExpanded(true);
                          pins.toggle(row);
                        }}
                      >
                        {row.content}
                      </EntityPinRow>
                    )),
                  ]
                : [];
            })}
          </ul>
        </div>

        {}
        <div className={styles.footer}>
          <Button
            variant="tertiaryColor"
            size="sm"
            className={styles.footerButton}
            onClick={() => {
              close();
              onCreateAgent();
            }}
          >
            <Plus aria-hidden="true" size={15} strokeWidth={1.9} />
            <span>{t("iaTargetCreateAgent")}</span>
          </Button>
          <Button
            variant="tertiaryColor"
            size="sm"
            className={styles.footerButton}
            onClick={() => {
              close();
              onCreateTeam();
            }}
          >
            <Plus aria-hidden="true" size={15} strokeWidth={1.9} />
            <span>{t("iaTargetCreateTeam")}</span>
          </Button>
        </div>
      </PopoverContent>
    </PopoverRoot>
  );
}
