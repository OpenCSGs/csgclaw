import { describe, expect, it } from "vitest";
import { resolveTeamAvatarMembers, type WorkspaceTeam } from "@/models/tasks";
import type { AgentLike } from "@/models/agents";

const baseTeam: WorkspaceTeam = {
  id: "team-1",
  title: "Team 1",
  lead_agent_id: "u-manager",
  member_agent_ids: ["u-worker", "u-reviewer"],
  status: "active",
  created_at: "",
  updated_at: "",
};

function createAgent(id: string, avatar = "", name = id): AgentLike {
  return {
    id,
    name,
    avatar,
  } as AgentLike;
}

describe("resolveTeamAvatarMembers", () => {
  it("prefers members with configured avatars and keeps the lead first", () => {
    const members = resolveTeamAvatarMembers(baseTeam, [
      createAgent("u-worker", ""),
      createAgent("u-reviewer", "avatar/pic-2.png"),
      createAgent("u-manager", "avatar/3D-1.png"),
    ]);

    expect(members).toHaveLength(2);
    expect(members.map((member) => member.id)).toEqual(["u-manager", "u-reviewer"]);
    expect(members.every((member) => member.avatar)).toBe(true);
  });

  it("falls back to member initials only when no configured avatar exists", () => {
    const members = resolveTeamAvatarMembers(baseTeam, [
      createAgent("u-worker", ""),
      createAgent("u-reviewer", ""),
      createAgent("u-manager", ""),
    ]);

    expect(members.map((member) => member.id)).toEqual(["u-worker", "u-reviewer", "u-manager"]);
  });

  it("matches the lead avatar through local identity aliases", () => {
    const members = resolveTeamAvatarMembers(
      {
        ...baseTeam,
        lead_agent_id: "manager",
        member_agent_ids: ["u-manager"],
      },
      [createAgent("u-manager", "avatar/3D-1.png", "Manager")],
    );

    expect(members).toHaveLength(1);
    expect(members[0]).toMatchObject({
      id: "manager",
      avatar: "avatar/3D-1.png",
      name: "Manager",
    });
  });

  it("uses avatars from matched users when the agent has no direct avatar", () => {
    const members = resolveTeamAvatarMembers(
      {
        ...baseTeam,
        lead_agent_id: "manager",
        member_agent_ids: ["u-manager"],
      },
      [createAgent("u-manager", "", "Manager")],
      new Map([
        [
          "user-manager",
          {
            id: "user-manager",
            name: "Manager",
            avatar: "avatar/pic-3.png",
          },
        ],
      ]),
    );

    expect(members).toHaveLength(1);
    expect(members[0]).toMatchObject({
      id: "manager",
      avatar: "avatar/pic-3.png",
      name: "Manager",
    });
  });
});
