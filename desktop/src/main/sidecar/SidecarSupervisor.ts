import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { randomBytes, randomUUID } from "node:crypto";
import { EventEmitter } from "node:events";
import fs from "node:fs";
import path from "node:path";
import readline from "node:readline";
import { app } from "electron";
import { resolveSidecarEnvironment } from "./childEnvironment";
import { DesktopMessageType, DESKTOP_PROTOCOL_VERSION, type DesktopBootstrapMessage, type DesktopReadyMessage } from "./contract";
import { locateBackendBundle } from "./locateBundle";
import { assertCompatibleVersions, parseReadyMessage } from "./readyProtocol";

export type SidecarState =
  | "idle"
  | "starting"
  | "ready"
  | "stopping"
  | "stopped"
  | "crashed"
  | "retrying"
  | "failed";

export type SidecarConnection = {
  ready: DesktopReadyMessage;
  sessionToken: string;
};

const READY_TIMEOUT_MS = 30_000;
const GRACEFUL_SHUTDOWN_TIMEOUT_MS = 45_000;
const TERMINATE_TIMEOUT_MS = 2_000;
const MAX_STDERR_SUMMARY_BYTES = 12 * 1024;

export class SidecarSupervisor extends EventEmitter {
  private child: ChildProcessWithoutNullStreams | null = null;
  private connectionValue: SidecarConnection | null = null;
  private expectedStop = false;
  private stderrSummary = "";
  private stateValue: SidecarState = "idle";
  private childEnvironmentPromise: Promise<NodeJS.ProcessEnv> | null = null;
  readonly logPath: string;

  constructor() {
    super();
    this.logPath = path.join(app.getPath("logs"), "backend.log");
  }

  get state(): SidecarState {
    return this.stateValue;
  }

  get connection(): SidecarConnection {
    if (!this.connectionValue) {
      throw new Error("Go sidecar is not ready.");
    }
    return this.connectionValue;
  }

  get failureSummary(): string {
    return this.stderrSummary.trim() || "The Go sidecar stopped before reporting a diagnostic message.";
  }

  async startWithRetry(maxAttempts = 3): Promise<SidecarConnection> {
    let lastError: unknown;
    for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
      try {
        return await this.start();
      } catch (error) {
        lastError = error;
        await this.stopOwnedProcess();
        if (attempt >= maxAttempts) {
          break;
        }
        this.setState("retrying");
        await delay(Math.min(250 * 2 ** (attempt - 1), 2_000));
      }
    }
    this.setState("failed");
    throw lastError instanceof Error ? lastError : new Error(String(lastError));
  }

  async start(): Promise<SidecarConnection> {
    if (this.connectionValue && this.child && this.stateValue === "ready") {
      return this.connectionValue;
    }
    if (this.child) {
      throw new Error(`Cannot start Go sidecar while it is ${this.stateValue}.`);
    }

    this.setState("starting");
    this.expectedStop = false;
    this.connectionValue = null;
    this.stderrSummary = "";

    const bundle = locateBackendBundle();
    const instanceID = randomUUID();
    const sessionToken = randomBytes(32).toString("base64url");
    const args = ["_desktop-serve"];
    const configPath = process.env.CSGCLAW_DESKTOP_CONFIG?.trim();
    if (configPath) {
      args.push("--config", configPath);
    }
    const childEnvironment = await this.resolveChildEnvironment();
    const child = spawn(bundle.executable, args, {
      cwd: bundle.root,
      env: childEnvironment,
      stdio: ["pipe", "pipe", "pipe"],
      windowsHide: true,
    });
    this.child = child;
    this.pipeBackendLog(child);

    const bootstrap: DesktopBootstrapMessage = {
      type: DesktopMessageType.bootstrap,
      protocol_version: DESKTOP_PROTOCOL_VERSION,
      instance_id: instanceID,
      session_token: sessionToken,
    };
    child.stdin.write(`${JSON.stringify(bootstrap)}\n`);

    return new Promise<SidecarConnection>((resolve, reject) => {
      let startupSettled = false;
      const output = readline.createInterface({ input: child.stdout, crlfDelay: Infinity });
      const timeout = setTimeout(() => {
        failStartup(new Error(`Go sidecar did not become ready within ${READY_TIMEOUT_MS / 1000} seconds.`));
      }, READY_TIMEOUT_MS);

      const failStartup = (error: Error) => {
        if (startupSettled) {
          return;
        }
        startupSettled = true;
        clearTimeout(timeout);
        output.close();
        reject(error);
      };

      output.on("line", (line) => {
        if (startupSettled || !line.trim()) {
          return;
        }
        // Embedded dependencies may print human-readable startup notices to
        // stdout before the desktop protocol message. Reserve JSON objects for
        // the protocol and keep waiting when a line is plainly diagnostic text.
        if (!line.trimStart().startsWith("{")) {
          return;
        }
        try {
          const ready = parseReadyMessage(line, instanceID, child.pid ?? -1);
          assertCompatibleVersions(app.getVersion(), ready.version, app.isPackaged);
          startupSettled = true;
          clearTimeout(timeout);
          output.close();
          const connection = { ready, sessionToken };
          this.connectionValue = connection;
          this.setState("ready");
          resolve(connection);
        } catch (error) {
          failStartup(error instanceof Error ? error : new Error(String(error)));
        }
      });

      child.once("error", (error) => {
        failStartup(new Error(`Failed to launch Go sidecar: ${error.message}`));
      });
      child.once("exit", (code, signal) => {
        const wasReady = this.connectionValue !== null;
        const wasExpected = this.expectedStop;
        this.child = null;
        this.connectionValue = null;
        if (!startupSettled) {
          failStartup(
            new Error(
              `Go sidecar exited before ready (code=${String(code)}, signal=${String(signal)}).\n${this.failureSummary}`,
            ),
          );
          return;
        }
        if (wasExpected) {
          this.setState("stopped");
          return;
        }
        if (wasReady) {
          this.setState("crashed");
          this.emit("crashed", new Error(this.failureSummary));
        }
      });
    });
  }

  async stop(reason = "app-quit"): Promise<void> {
    const child = this.child;
    if (!child) {
      this.connectionValue = null;
      this.setState("stopped");
      return;
    }
    this.expectedStop = true;
    this.setState("stopping");
    const exitPromise = waitForExit(child);
    if (child.stdin.writable) {
      child.stdin.write(`${JSON.stringify({ type: DesktopMessageType.shutdown, reason })}\n`);
    }
    if (await resolvesWithin(exitPromise, GRACEFUL_SHUTDOWN_TIMEOUT_MS)) {
      return;
    }
    child.kill("SIGTERM");
    if (await resolvesWithin(exitPromise, TERMINATE_TIMEOUT_MS)) {
      return;
    }
    child.kill("SIGKILL");
    await resolvesWithin(exitPromise, TERMINATE_TIMEOUT_MS);
  }

  async restart(reason = "desktop-restart"): Promise<SidecarConnection> {
    await this.stop(reason);
    this.childEnvironmentPromise = null;
    return this.startWithRetry();
  }

  private async stopOwnedProcess(): Promise<void> {
    if (!this.child) {
      this.connectionValue = null;
      return;
    }
    await this.stop("startup-failed");
  }

  private pipeBackendLog(child: ChildProcessWithoutNullStreams): void {
    fs.mkdirSync(path.dirname(this.logPath), { recursive: true, mode: 0o700 });
    const log = fs.createWriteStream(this.logPath, { flags: "a", mode: 0o600 });
    log.write(`\n[${new Date().toISOString()}] starting Go sidecar\n`);
    child.stdout.on("data", (chunk: Buffer | string) => {
      log.write(`[stdout] ${chunk.toString()}`);
    });
    child.stderr.on("data", (chunk: Buffer | string) => {
      const text = chunk.toString();
      log.write(text);
      this.stderrSummary = `${this.stderrSummary}${text}`.slice(-MAX_STDERR_SUMMARY_BYTES);
    });
    child.once("close", () => {
      log.end();
    });
  }

  private resolveChildEnvironment(): Promise<NodeJS.ProcessEnv> {
    this.childEnvironmentPromise ??= resolveSidecarEnvironment({
      homeDirectory: app.getPath("home"),
    });
    return this.childEnvironmentPromise;
  }

  private setState(next: SidecarState): void {
    if (this.stateValue === next) {
      return;
    }
    this.stateValue = next;
    this.emit("state", next);
  }
}

function waitForExit(child: ChildProcessWithoutNullStreams): Promise<void> {
  return new Promise((resolve) => {
    if (child.exitCode !== null || child.signalCode !== null) {
      resolve();
      return;
    }
    child.once("exit", () => resolve());
  });
}

async function resolvesWithin(promise: Promise<void>, timeoutMS: number): Promise<boolean> {
  return Promise.race([promise.then(() => true), delay(timeoutMS).then(() => false)]);
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}
