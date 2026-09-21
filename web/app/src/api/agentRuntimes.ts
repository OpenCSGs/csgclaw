import { get, post, type ApiError } from "@/api/client";
import { ApiEndpoints } from "@/shared/constants/api";

export type AgentRuntimeInstallation = {
  name: string;
  status: "idle" | "running" | "succeeded" | "failed";
  stage: string;
  activityCount: number;
  startedAt: string;
  updatedAt: string;
  runtime?: unknown;
  errorCode?: string;
  message?: string;
};

export type AgentRuntimeInstallProgress = (installation: AgentRuntimeInstallation) => void;

export function fetchAgentRuntimes(): Promise<unknown> {
  return get(ApiEndpoints.agentRuntimes, { cache: "no-store" });
}

export async function installAgentRuntime(name: string, onProgress?: AgentRuntimeInstallProgress): Promise<unknown> {
  let installation = normalizeInstallation(
    await post(`${ApiEndpoints.agentRuntimes}/${encodeURIComponent(name)}/install`),
  );
  onProgress?.(installation);
  while (installation.status === "running") {
    await wait(500);
    installation = normalizeInstallation(
      await get(`${ApiEndpoints.agentRuntimes}/${encodeURIComponent(name)}/install`, { cache: "no-store" }),
    );
    onProgress?.(installation);
  }
  if (installation.status === "failed") {
    const error: ApiError = {
      status: 500,
      code: installation.errorCode || "dsh_install_failed",
      message: installation.message || "DeepSeek Harness installation failed",
    };
    throw error;
  }
  if (installation.status !== "succeeded" || !installation.runtime) {
    throw {
      status: 500,
      code: "dsh_install_failed",
      message: "DeepSeek Harness installation did not return a runtime",
    } satisfies ApiError;
  }
  return installation.runtime;
}

function normalizeInstallation(value: unknown): AgentRuntimeInstallation {
  const record = value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
  const rawStatus = String(record.status || "idle").trim();
  const status = rawStatus === "running" || rawStatus === "succeeded" || rawStatus === "failed" ? rawStatus : "idle";
  return {
    name: String(record.name || "").trim(),
    status,
    stage: String(record.stage || "").trim(),
    activityCount: Math.max(0, Math.floor(Number(record.activity_count) || 0)),
    startedAt: String(record.started_at || "").trim(),
    updatedAt: String(record.updated_at || "").trim(),
    runtime: record.runtime,
    errorCode: String(record.error_code || "").trim() || undefined,
    message: String(record.message || "").trim() || undefined,
  };
}

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}
