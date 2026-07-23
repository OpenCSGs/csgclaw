import { useEffect, useState } from "react";
import type { ConversationWorkingParticipant } from "@/components/business/ConversationPane";
import {
  agentActivityToolMergeKey,
  isTerminalAgentActivityTool,
  parseAgentActivity,
  parseMessageActivityCommand,
  questionActivityKeepsAgentWorking,
} from "@/models/agentActivity";
import { agentRuntimeKind, type AgentLike } from "@/models/agents";
import {
  agentMatchesUser,
  isDirectConversation,
  isThreadReply,
  isToolCallMessage,
  localIdentitiesMatch,
  resolveUserByLocalIdentity,
} from "@/models/conversations";
import type { IMConversation, IMMessage, UsersById } from "@/models/conversations";
import { isNewConversationSlashCommand } from "@/models/slashCommands";
import { AgentActivityKinds, AgentActivityMsgTypes } from "@/shared/constants/messages";

// Temporary compatibility for OpenClaw images that predate participant work leases.
// Remove this module and its single controller call after those images are retired.
const LEGACY_WORKING_TIMEOUT_MS = 120_000;
const LEGACY_QUESTION_CONTINUATION_TIMEOUT_MS = 10 * 60_000;
const PARTICIPANT_ACTIVITY_TURN_PLACEHOLDER = "\u200b";

type LegacyOpenClawWorkingArgs = {
  agents: readonly AgentLike[];
  authoritative: readonly ConversationWorkingParticipant[];
  conversation: IMConversation | null | undefined;
  currentUserID: string | null | undefined;
  hasObservedWorkLease: (participantID: string | null | undefined) => boolean;
  now?: number;
  usersById: UsersById;
};

type LegacyWorkingResult = {
  nextDeadline: number | null;
  participants: ConversationWorkingParticipant[];
};

export function useLegacyOpenClawWorkingFallback(
  args: Omit<LegacyOpenClawWorkingArgs, "now">,
): ConversationWorkingParticipant[] {
  const [now, setNow] = useState(() => Date.now());
  const result = legacyOpenClawWorkingResult({ ...args, now: Math.max(now, Date.now()) });

  useEffect(() => {
    if (result.nextDeadline === null) {
      return;
    }
    const timer = window.setTimeout(() => setNow(Date.now()), Math.max(0, result.nextDeadline - Date.now()));
    return () => window.clearTimeout(timer);
  }, [result.nextDeadline]);

  return result.participants;
}

export function withLegacyOpenClawWorkingFallback({
  agents,
  authoritative,
  conversation,
  currentUserID,
  hasObservedWorkLease,
  now = Date.now(),
  usersById,
}: LegacyOpenClawWorkingArgs): ConversationWorkingParticipant[] {
  return legacyOpenClawWorkingResult({
    agents,
    authoritative,
    conversation,
    currentUserID,
    hasObservedWorkLease,
    now,
    usersById,
  }).participants;
}

function legacyOpenClawWorkingResult({
  agents,
  authoritative,
  conversation,
  currentUserID,
  hasObservedWorkLease,
  now = Date.now(),
  usersById,
}: LegacyOpenClawWorkingArgs): LegacyWorkingResult {
  const targets = openClawTargetsForConversation(conversation, currentUserID, agents, usersById).filter(
    (target) => !hasObservedWorkLease(target.id),
  );
  if (!conversation || !targets.length) {
    return { nextDeadline: null, participants: mergeWorkingParticipants(authoritative) };
  }
  const pending = pendingMessageParticipants(conversation, currentUserID, targets, now);
  const tools = activeToolParticipants(conversation, currentUserID, targets, now);
  const deadlines = [pending.nextDeadline, tools.nextDeadline].filter((value): value is number => value !== null);
  return {
    nextDeadline: deadlines.length ? Math.min(...deadlines) : null,
    participants: mergeWorkingParticipants(authoritative, pending.participants, tools.participants),
  };
}

function openClawTargetsForConversation(
  conversation: IMConversation | null | undefined,
  currentUserID: string | null | undefined,
  agents: readonly AgentLike[],
  usersById: UsersById,
): ConversationWorkingParticipant[] {
  if (!conversation || !currentUserID) {
    return [];
  }

  const byID = new Map<string, ConversationWorkingParticipant>();
  conversation.members.forEach((memberID) => {
    if (!memberID || localIdentitiesMatch(memberID, currentUserID)) {
      return;
    }
    const user = resolveUserByLocalIdentity(memberID, usersById);
    const agent = agents.find(
      (candidate) =>
        // Agent metadata does not expose a work-lease capability bit, so the
        // temporary fallback is intentionally scoped to the whole runtime.
        agentRuntimeKind(candidate) === "openclaw_sandbox" &&
        (localIdentitiesMatch(candidate.id, memberID) ||
          localIdentitiesMatch(candidate.user_id, memberID) ||
          (user ? agentMatchesUser(candidate, user) : false)),
    );
    if (!agent) {
      return;
    }
    const id = String(user?.id || agent.user_id || agent.id || memberID).trim();
    const name = String(agent.name || user?.name || id).trim();
    if (id && name) {
      byID.set(id, { id, name });
    }
  });
  return Array.from(byID.values()).sort((left, right) => left.name.localeCompare(right.name));
}

function pendingMessageParticipants(
  conversation: IMConversation,
  currentUserID: string | null | undefined,
  targets: readonly ConversationWorkingParticipant[],
  now: number,
): LegacyWorkingResult {
  const currentID = String(currentUserID || "").trim();
  if (!currentID) {
    return { nextDeadline: null, participants: [] };
  }

  const repliedTargetIDs = new Set<string>();
  let questionContinuationAt: number | null = null;
  for (let index = conversation.messages.length - 1; index >= 0; index -= 1) {
    const message = conversation.messages[index];
    if (isThreadReply(message) || isToolCallMessage(message) || isTurnPlaceholder(message)) {
      continue;
    }
    if (localIdentitiesMatch(message.sender_id, currentID) && isNewConversationSlashCommand(message.content)) {
      return { nextDeadline: null, participants: [] };
    }
    if (questionActivityKeepsAgentWorking(message)) {
      const question = parseAgentActivity(message)?.content.question;
      const continuationAt = Date.parse(
        String(question?.resolved_at || question?.requested_at || message.created_at || ""),
      );
      if (Number.isFinite(continuationAt)) {
        questionContinuationAt = Math.max(questionContinuationAt ?? continuationAt, continuationAt);
      }
      continue;
    }

    const senderID = String(message.sender_id || "").trim();
    const replyTarget = targets.find((target) => localIdentitiesMatch(target.id, senderID));
    if (replyTarget) {
      repliedTargetIDs.add(replyTarget.id);
      continue;
    }
    if (!localIdentitiesMatch(senderID, currentID)) {
      continue;
    }

    const pending = targetsForStoredMessage(conversation, message, targets).filter(
      (target) => !Array.from(repliedTargetIDs).some((id) => localIdentitiesMatch(id, target.id)),
    );
    if (!pending.length) {
      continue;
    }
    const deadline =
      questionContinuationAt === null
        ? messageWorkingDeadline(message)
        : questionContinuationAt + LEGACY_QUESTION_CONTINUATION_TIMEOUT_MS;
    const scopedPending = pending.map((target) => ({
      ...target,
      activityAfter: message.created_at || "",
      ...(message.id ? { requestID: message.id } : {}),
    }));
    return deadline !== null && deadline > now
      ? { nextDeadline: deadline, participants: scopedPending }
      : { nextDeadline: null, participants: [] };
  }
  return { nextDeadline: null, participants: [] };
}

function activeToolParticipants(
  conversation: IMConversation,
  currentUserID: string | null | undefined,
  targets: readonly ConversationWorkingParticipant[],
  now: number,
): LegacyWorkingResult {
  const activeToolKeys = new Map<string, Set<string>>();
  const activeCommandCounts = new Map<string, Map<string, number>>();
  const latestSignalAt = new Map<string, number>();
  const latestRequestID = new Map<string, string>();

  conversation.messages.forEach((message) => {
    if (
      !isThreadReply(message) &&
      localIdentitiesMatch(message.sender_id, currentUserID) &&
      isNewConversationSlashCommand(message.content)
    ) {
      activeToolKeys.clear();
      activeCommandCounts.clear();
      latestSignalAt.clear();
      latestRequestID.clear();
      return;
    }
    const activity = parseAgentActivity(message);
    const tool = activity?.content.tool;
    const target = activityTargetForMessage(message, activity?.sender, targets);

    if (activity?.content.msgtype === AgentActivityMsgTypes.tool && tool) {
      if (!target) {
        return;
      }
      const toolKey = agentActivityToolMergeKey(tool);
      if (!toolKey) {
        return;
      }
      const keys = activeToolKeys.get(target.id) ?? new Set<string>();
      if (isTerminalAgentActivityTool(tool)) {
        keys.delete(toolKey);
      } else {
        keys.add(toolKey);
      }
      if (keys.size) activeToolKeys.set(target.id, keys);
      else activeToolKeys.delete(target.id);
      recordLatestSignal(latestSignalAt, target.id, message);
      recordLatestRequest(latestRequestID, target.id, message);
      return;
    }

    const command = parseMessageActivityCommand(message);
    if (command?.kind === AgentActivityKinds.execCommand && target) {
      const counts = activeCommandCounts.get(target.id) ?? new Map<string, number>();
      const count = counts.get(command.signature) ?? 0;
      if (command.output) {
        if (count > 1) counts.set(command.signature, count - 1);
        else counts.delete(command.signature);
      } else {
        counts.set(command.signature, count + 1);
      }
      if (counts.size) activeCommandCounts.set(target.id, counts);
      else activeCommandCounts.delete(target.id);
      recordLatestSignal(latestSignalAt, target.id, message);
      recordLatestRequest(latestRequestID, target.id, message);
      return;
    }

    if (target && !isToolCallMessage(message) && !isTurnPlaceholder(message)) {
      activeToolKeys.delete(target.id);
      activeCommandCounts.delete(target.id);
      latestSignalAt.delete(target.id);
      latestRequestID.delete(target.id);
    }
  });

  const deadlines: number[] = [];
  const participants = targets.flatMap((target) => {
    if (!activeToolKeys.has(target.id) && !activeCommandCounts.has(target.id)) {
      return [];
    }
    const signalAt = latestSignalAt.get(target.id);
    if (signalAt === undefined) {
      return [];
    }
    const deadline = signalAt + LEGACY_WORKING_TIMEOUT_MS;
    if (deadline <= now) {
      return [];
    }
    deadlines.push(deadline);
    const requestID = latestRequestID.get(target.id);
    return [
      {
        ...target,
        activityAfter: new Date(signalAt).toISOString(),
        ...(requestID ? { requestID } : {}),
      },
    ];
  });
  return { nextDeadline: deadlines.length ? Math.min(...deadlines) : null, participants };
}

function recordLatestSignal(target: Map<string, number>, participantID: string, message: IMMessage): void {
  const createdAt = Date.parse(String(message.created_at || ""));
  if (Number.isFinite(createdAt)) {
    target.set(participantID, createdAt);
  }
}

function recordLatestRequest(target: Map<string, string>, participantID: string, message: IMMessage): void {
  const metadata = asRecord(message.metadata);
  const openclaw = asRecord(metadata?.openclaw);
  const codex = asRecord(metadata?.codex);
  const requestID = firstText(
    openclaw?.request_id,
    openclaw?.requestId,
    openclaw?.source_message_id,
    codex?.request_id,
    codex?.requestId,
  );
  if (requestID) {
    target.set(participantID, requestID);
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

function firstText(...values: unknown[]): string {
  for (const value of values) {
    const text = String(value || "").trim();
    if (text) {
      return text;
    }
  }
  return "";
}

function activityTargetForMessage(
  message: IMMessage,
  activitySender: string | null | undefined,
  targets: readonly ConversationWorkingParticipant[],
): ConversationWorkingParticipant | undefined {
  return targets.find((target) =>
    [message.sender_id, activitySender].some((senderID) => localIdentitiesMatch(senderID, target.id)),
  );
}

function targetsForStoredMessage(
  conversation: IMConversation,
  message: IMMessage,
  targets: readonly ConversationWorkingParticipant[],
): ConversationWorkingParticipant[] {
  if (isDirectConversation(conversation)) {
    return [...targets];
  }
  const mentionedIDs = mentionedIDsFromMessage(message);
  return targets.filter((target) => mentionedIDs.some((id) => localIdentitiesMatch(id, target.id)));
}

function mentionedIDsFromMessage(message: IMMessage): string[] {
  const ids = new Set<string>();
  (message.mentions || []).forEach((mention) => {
    const id = typeof mention === "string" ? mention : mention?.id;
    const normalized = String(id || "").trim();
    if (normalized) {
      ids.add(normalized);
    }
  });
  for (const match of String(message.content || "").matchAll(/<at\s+[^>]*user_id=["']([^"']+)["'][^>]*>/g)) {
    const id = String(match[1] || "").trim();
    if (id) {
      ids.add(id);
    }
  }
  return Array.from(ids);
}

function messageWorkingDeadline(message: IMMessage): number | null {
  const createdAt = Date.parse(String(message.created_at || ""));
  return Number.isFinite(createdAt) ? createdAt + LEGACY_WORKING_TIMEOUT_MS : null;
}

function isTurnPlaceholder(message: IMMessage | null | undefined): boolean {
  return String(message?.content || "") === PARTICIPANT_ACTIVITY_TURN_PLACEHOLDER;
}

function mergeWorkingParticipants(
  ...groups: readonly (readonly ConversationWorkingParticipant[] | null | undefined)[]
): ConversationWorkingParticipant[] {
  const merged: ConversationWorkingParticipant[] = [];
  groups.forEach((group) => {
    (group || []).forEach((participant) => {
      const id = String(participant.id || "").trim();
      const name = String(participant.name || id).trim();
      if (!id || !name) {
        return;
      }
      const existing = merged.findIndex((candidate) => localIdentitiesMatch(candidate.id, id));
      if (existing >= 0) {
        merged[existing] = {
          ...merged[existing],
          ...participant,
          id: merged[existing].id,
          name: merged[existing].name || name,
        };
      } else {
        merged.push({ ...participant, id, name });
      }
    });
  });
  return merged.sort((left, right) => left.name.localeCompare(right.name));
}
