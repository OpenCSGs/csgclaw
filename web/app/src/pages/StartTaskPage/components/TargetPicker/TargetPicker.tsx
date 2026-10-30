import { useMemo, useRef, useState } from "react";
import { Check, ChevronDown, Plus, Search } from "lucide-react";
import { Button, PopoverContent, PopoverRoot, PopoverTrigger } from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { RoomAvatar } from "@/components/business/RoomAvatar";
import { agentRoleLabel, isManagerAgent as isManagerAgentItem } from "@/models/agents";
import { resolveTeamAvatarMembers } from "@/models/tasks";
import { classNames } from "@/shared/lib/classNames";
import styles from "./TargetPicker.module.css";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn, UsersById } from "@/models/conversations";
import type { WorkspaceTeam } from "@/models/tasks";

export type TargetKind = "agent" | "team";

export const SOLO_TEAM_ID = "__solo__";

export type Target = {
  kind: TargetKind;
  id: string;
};

export type TargetPickerProps = {
  agents: readonly AgentLike[];
  teams: readonly WorkspaceTeam[];
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
  teams,
  usersById,
  value,
  locked = false,
  t,
  onChange,
  onCreateAgent,
  onCreateTeam,
}: TargetPickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement | null>(null);

  const managerAgent = useMemo(() => agents.find((item) => isManagerAgentItem(item)) ?? agents[0] ?? null, [agents]);

  const isSolo = value.kind === "team" && value.id === SOLO_TEAM_ID;
  const selectedAgent = value.kind === "agent" ? (agents.find((item) => item.id === value.id) ?? null) : null;
  const selectedTeam = value.kind === "team" && !isSolo ? (teams.find((item) => item.id === value.id) ?? null) : null;

  const fallbackAgent = agents[0] ?? null;
  const triggerName = isSolo
    ? t("iaStartRoomSolo")
    : selectedAgent?.name || selectedTeam?.title || fallbackAgent?.name || "";

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
                avatar={selectedAgent?.avatar ?? fallbackAgent?.avatar}
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
          <input
            ref={searchRef}
            aria-label={t("iaTargetSearch")}
            className={styles.searchInput}
            placeholder={t("iaTargetSearch")}
            type="text"
            value={query}
            onChange={(event) => setQuery(event.currentTarget.value)}
          />
        </div>

        <div className={styles.list}>
          {visibleAgents.length === 0 && visibleTeams.length === 0 ? (
            <p className={styles.empty}>{t("iaTargetEmpty")}</p>
          ) : null}

          {visibleAgents.length > 0 ? (
            <section className={styles.group}>
              <div className={styles.groupLabel}>{t("iaTargetGroupAgents")}</div>
              <ul className={styles.items}>
                {visibleAgents.map((item) => {
                  const id = String(item.id || "");
                  const active = value.kind === "agent" && value.id === id;
                  return (
                    <li key={id}>
                      <button
                        type="button"
                        className={classNames(styles.row, active && styles.rowActive)}
                        onClick={() => pick({ kind: "agent", id })}
                      >
                        {active ? (
                          <Check aria-hidden="true" className={styles.rowCheck} size={15} strokeWidth={2.4} />
                        ) : null}
                        <span className={classNames("iaAvatarFrame", styles.rowIcon)} aria-hidden="true">
                          <AgentAvatarContent avatar={item.avatar} fallback={String(item.name || id).slice(0, 1)} />
                        </span>
                        <span className={styles.rowText}>
                          <span className={styles.rowName}>{item.name || id}</span>
                        </span>
                        {}
                        <span className={styles.rowBadge}>{agentRoleLabel(item, t)}</span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </section>
          ) : null}

          {}
          {visibleTeams.length > 0 || !needle ? (
            <section className={styles.group}>
              <div className={styles.groupLabel}>{t("iaTargetGroupTeams")}</div>
              <ul className={styles.items}>
                {}
                {!needle ? (
                  <li>
                    <button
                      type="button"
                      className={classNames(
                        styles.row,
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
                  </li>
                ) : null}
                {visibleTeams.map((team) => {
                  const active = value.kind === "team" && value.id === team.id;
                  return (
                    <li key={team.id}>
                      <button
                        type="button"
                        className={classNames(styles.row, active && styles.rowActive)}
                        onClick={() => pick({ kind: "team", id: team.id })}
                      >
                        {active ? (
                          <Check aria-hidden="true" className={styles.rowCheck} size={15} strokeWidth={2.4} />
                        ) : null}
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
                        <span className={styles.rowBadge}>
                          {t("iaTargetTeamMembers", { count: team.member_agent_ids.length })}
                        </span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </section>
          ) : null}
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
