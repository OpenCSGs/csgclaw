import { post } from "@/api/client";
import type { ApiError } from "@/api/client";

export type ActivityDecision = {
  decided_at?: string;
  kind?: string;
  option_id?: string;
};

export type ActivitySnapshot = {
  decision?: ActivityDecision | null;
  expires_at?: string;
  id: string;
  kind?: string;
  options?: Array<{ id: string; kind: string; label: string; scope?: string }>;
  requested_at?: string;
  status: string;
  title?: string;
};

export type UserInputAnswer = {
  answers: string[];
};

export type UserInputResponse = {
  answers: Record<string, UserInputAnswer>;
};

export type UserInputSnapshot = {
  answers?: Record<
    string,
    {
      answered?: boolean;
      option_index?: number;
      option_label?: string;
      secret?: boolean;
      skipped?: boolean;
      text?: string;
    }
  >;
  auto_resolve_at?: string;
  channel?: string;
  id: string;
  questions: Array<{
    header: string;
    id: string;
    is_other?: boolean;
    is_secret?: boolean;
    options?: Array<{ description?: string; label: string }>;
    question: string;
  }>;
  requested_at?: string;
  resolved_at?: string;
  responder_id?: string;
  room_id?: string;
  status: string;
};

export async function decideChannelActivity(
  channel: string,
  activityID: string,
  optionID: string,
): Promise<ActivitySnapshot> {
  try {
    return await post<ActivitySnapshot>(
      `api/v1/channels/${encodeURIComponent(channel)}/activities/${encodeURIComponent(activityID)}:decide`,
      { option_id: optionID },
    );
  } catch (error) {
    const maybeSnapshot = snapshotFromAPIError(error);
    if (maybeSnapshot) {
      return maybeSnapshot;
    }
    throw error;
  }
}

export async function respondToUserInput(
  channel: string,
  activityID: string,
  response: UserInputResponse,
): Promise<UserInputSnapshot> {
  try {
    return await post<UserInputSnapshot>(
      `api/v1/channels/${encodeURIComponent(channel)}/activities/${encodeURIComponent(activityID)}:respond`,
      response,
    );
  } catch (error) {
    const maybeSnapshot = userInputSnapshotFromAPIError(error);
    if (maybeSnapshot) {
      return maybeSnapshot;
    }
    throw error;
  }
}

function snapshotFromAPIError(error: unknown): ActivitySnapshot | null {
  const apiError = error as ApiError | null;
  if (!apiError || (apiError.status !== 409 && apiError.status !== 410)) {
    return null;
  }
  try {
    const parsed = JSON.parse(apiError.message) as Partial<ActivitySnapshot>;
    if (typeof parsed?.id === "string" && typeof parsed?.status === "string") {
      return parsed as ActivitySnapshot;
    }
  } catch {
    return null;
  }
  return null;
}

function userInputSnapshotFromAPIError(error: unknown): UserInputSnapshot | null {
  const apiError = error as ApiError | null;
  if (!apiError || (apiError.status !== 409 && apiError.status !== 410)) {
    return null;
  }
  try {
    const parsed = JSON.parse(apiError.message) as Partial<UserInputSnapshot>;
    if (typeof parsed?.id === "string" && typeof parsed?.status === "string") {
      return parsed as UserInputSnapshot;
    }
  } catch {
    return null;
  }
  return null;
}
