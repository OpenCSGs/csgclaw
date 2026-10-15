import { EventEmitter } from "node:events";
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { randomUUID } from "node:crypto";
import path from "node:path";
import { app } from "electron";
import type { DesktopTurnNotification } from "../shared/desktopBridge.types";
import { logDesktopError } from "./desktopLogger";

export type MacNotificationPermission =
  | "default"
  | "granted"
  | "denied"
  | "unsupported";
class NotificationServiceStoppedError extends Error {
  constructor() {
    super("Notification service stopped.");
  }
}
type Packet = Record<string, unknown>;
type Pending = {
  resolve: (packet: Packet) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
};
type Launch = (executable: string) => ChildProcessWithoutNullStreams;

export class MacNotificationService {
  private child: ChildProcessWithoutNullStreams | null = null;
  private pending = new Map<string, Pending>();
  private clicks = new Map<string, () => void>();

  constructor(
    private readonly executable: string,
    private readonly launch: Launch = (file) =>
      spawn(file, [], { stdio: "pipe" }),
  ) {}

  watch(id: string, onClick: () => void): () => void {
    this.clicks.set(id, onClick);
    return () => this.clicks.delete(id);
  }

  async permission(request = false): Promise<MacNotificationPermission> {
    const reply = await this.request(
      request ? "request_permission" : "permission",
    );
    if (
      !["default", "granted", "denied", "unsupported"].includes(
        String(reply.permission),
      )
    )
      throw new Error("Invalid native notification permission.");
    return reply.permission as MacNotificationPermission;
  }

  async show(notification: DesktopTurnNotification): Promise<void> {
    await this.request("show", { notification });
  }

  async close(id: string): Promise<void> {
    await this.request("close", { notification_id: id });
  }

  stop(): void {
    const child = this.child;
    this.child = null;
    this.rejectAll(new NotificationServiceStoppedError());
    this.clicks.clear();
    child?.stdin.end();
    if (child) {
      const timer = setTimeout(() => child.kill(), 2000);
      timer.unref();
      child.once("exit", () => clearTimeout(timer));
    }
  }

  private request(operation: string, payload: Packet = {}): Promise<Packet> {
    return new Promise((resolve, reject) => {
      const child = this.start();
      const id = randomUUID();
      const timer = setTimeout(
        () => {
          this.pending.delete(id);
          reject(new Error("Native notification service timed out."));
        },
        operation === "request_permission" ? 120000 : 15000,
      );
      timer.unref();
      this.pending.set(id, { resolve, reject, timer });
      child.stdin.write(
        `${JSON.stringify({ request_id: id, operation, ...payload })}\n`,
        (error) => {
          if (!error) return;
          const pending = this.pending.get(id);
          if (pending) {
            clearTimeout(pending.timer);
            this.pending.delete(id);
            pending.reject(error);
          }
        },
      );
    });
  }

  private start(): ChildProcessWithoutNullStreams {
    if (this.child) return this.child;
    const child = this.launch(this.executable);
    this.child = child;
    let buffer = "";
    child.stdout.setEncoding("utf8");
    child.stderr.resume();
    const failed = (error: Error) => {
      if (this.child !== child) return;
      this.child = null;
      this.rejectAll(error);
      logDesktopError("notification-service-failed", error);
    };
    child.on("error", failed);
    child.on("exit", () =>
      failed(new Error("Native notification service exited.")),
    );
    child.stdin.on("error", failed);
    child.stdout.on("data", (data: string) => {
      buffer += data;
      let index: number;
      while ((index = buffer.indexOf("\n")) >= 0) {
        const line = buffer.slice(0, index);
        buffer = buffer.slice(index + 1);
        try {
          this.receive(JSON.parse(line) as Packet);
        } catch {
          failed(new Error("Invalid native notification service response."));
          child.kill();
          return;
        }
      }
      if (buffer.length > 65536) {
        failed(
          new Error("Native notification service response exceeded its limit."),
        );
        child.kill();
      }
    });
    return child;
  }

  private receive(packet: Packet): void {
    if (!packet || typeof packet !== "object" || Array.isArray(packet))
      throw new Error("Invalid response.");
    if (packet.event === "ready") return;
    if (
      packet.event === "click" &&
      typeof packet.notification_id === "string"
    ) {
      this.clicks.get(packet.notification_id)?.();
      return;
    }
    if (typeof packet.request_id !== "string")
      throw new Error("Missing request identity.");
    const pending = this.pending.get(packet.request_id);
    if (!pending) return;
    clearTimeout(pending.timer);
    this.pending.delete(packet.request_id);
    if (packet.ok === true) pending.resolve(packet);
    else
      pending.reject(
        new Error(
          typeof packet.error === "string"
            ? packet.error
            : "Native notification delivery failed.",
        ),
      );
  }

  private rejectAll(error: Error): void {
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timer);
      pending.reject(error);
    }
    this.pending.clear();
  }
}

let shared: MacNotificationService | undefined;
export function macNotificationService(): MacNotificationService {
  const root = app.isPackaged
    ? process.resourcesPath
    : path.resolve(__dirname, "..", "native");
  return (shared ??= new MacNotificationService(
    path.join(
      root,
      "CSGClaw Notifications.app",
      "Contents",
      "MacOS",
      "CSGClawNotifications",
    ),
  ));
}
export function stopMacNotificationService(): void {
  shared?.stop();
  shared = undefined;
}

export class MacNotification extends EventEmitter {
  private unwatch: (() => void) | undefined;
  private closed = false;
  constructor(
    private readonly input: DesktopTurnNotification,
    private readonly service = macNotificationService(),
  ) {
    super();
  }
  show(): void {
    this.unwatch = this.service.watch(this.input.id, () =>
      this.emit("click", {}),
    );
    void this.service.show(this.input).then(
      () => {
        if (!this.closed) this.emit("show", {});
      },
      (error: unknown) => {
        this.unwatch?.();
        this.emit(
          "failed",
          {},
          error instanceof Error
            ? error.message
            : "Native notification failed.",
        );
      },
    );
  }
  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.unwatch?.();
    void this.service.close(this.input.id).catch((error: unknown) => {
      if (!(error instanceof NotificationServiceStoppedError)) {
        logDesktopError("notification-close-failed", error);
      }
    });
    this.emit("close", {});
  }
}
