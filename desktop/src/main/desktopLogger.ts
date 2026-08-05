import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

const DEFAULT_MAX_LOG_BYTES = 2 * 1024 * 1024;

export type DesktopLogDetails = Record<string, boolean | number | string | null | undefined>;

let logPath = "";
let previousLogPath = "";
let maxLogBytes = DEFAULT_MAX_LOG_BYTES;
let runId = "";

export function initializeDesktopLogger(
  logDirectory: string,
  maximumLogBytes = DEFAULT_MAX_LOG_BYTES,
): string {
  logPath = path.join(logDirectory, "main.log");
  previousLogPath = path.join(logDirectory, "main.previous.log");
  maxLogBytes = maximumLogBytes;
  runId = randomUUID();
  return logPath;
}

export function logDesktopInfo(event: string, details: DesktopLogDetails = {}): void {
  writeDesktopLog("INFO", event, details);
}

export function logDesktopError(
  event: string,
  error: unknown,
  details: DesktopLogDetails = {},
): void {
  writeDesktopLog("ERROR", event, { ...details, ...describeError(error) });
}

function writeDesktopLog(
  level: "ERROR" | "INFO",
  event: string,
  details: DesktopLogDetails,
): void {
  if (!logPath) {
    return;
  }
  try {
    fs.mkdirSync(path.dirname(logPath), { recursive: true, mode: 0o700 });
    rotateDesktopLogIfNeeded();
    fs.appendFileSync(
      logPath,
      `${JSON.stringify({
        timestamp: new Date().toISOString(),
        level,
        event,
        ...details,
        pid: process.pid,
        runId,
      })}\n`,
      { encoding: "utf8", mode: 0o600 },
    );
  } catch {
    // Diagnostics must never become a new startup failure.
  }
}

function rotateDesktopLogIfNeeded(): void {
  try {
    if (fs.statSync(logPath).size < maxLogBytes) {
      return;
    }
    fs.rmSync(previousLogPath, { force: true });
    fs.renameSync(logPath, previousLogPath);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
      throw error;
    }
  }
}

export function describeError(error: unknown): DesktopLogDetails {
  if (error instanceof Error) {
    return {
      errorName: error.name,
      errorMessage: redactSensitiveText(error.message),
      errorStack: error.stack ? redactSensitiveText(error.stack) : undefined,
    };
  }
  return { errorMessage: redactSensitiveText(String(error)) };
}

export function diagnosticLaunchFlags(argv: readonly string[]): string {
  return [
    ...new Set(
      argv
        .filter((argument) => argument.startsWith("--"))
        .map((argument) => {
          const separator = argument.indexOf("=");
          return separator >= 0 ? argument.slice(0, separator) : argument;
        }),
    ),
  ].join(",");
}

function redactSensitiveText(value: string): string {
  return value
    .replace(/\bBearer\s+\S+/gi, "Bearer [redacted]")
    .replace(/\b(api[_-]?key|authorization|password|session[_-]?token|token)=([^\s&]+)/gi, "$1=[redacted]");
}
