import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { PassThrough } from "node:stream";
import test from "node:test";
import type { ChildProcessWithoutNullStreams } from "node:child_process";
import { MacNotificationService } from "./macNotificationService";

class HelperProcess extends EventEmitter {
  stdin = new PassThrough();
  stdout = new PassThrough();
  stderr = new PassThrough();
  requests: Array<Record<string, unknown>> = [];
  killed = false;
  constructor() {
    super();
    this.stdin.on("data", (data) =>
      this.requests.push(
        JSON.parse(data.toString()) as Record<string, unknown>,
      ),
    );
    this.stdin.on("finish", () => this.emit("exit", 0));
  }
  reply(index: number, value: Record<string, unknown>): void {
    const request = this.requests[index];
    assert.ok(request);
    this.stdout.write(
      `${JSON.stringify({ request_id: request.request_id, ...value })}\n`,
    );
  }
  kill(): boolean {
    this.killed = true;
    this.emit("exit", 1);
    return true;
  }
}

function fixture() {
  const children: HelperProcess[] = [];
  const service = new MacNotificationService(
    "/test/notification-helper",
    () => {
      const child = new HelperProcess();
      children.push(child);
      return child as unknown as ChildProcessWithoutNullStreams;
    },
  );
  const current = () => {
    const child = children.at(-1);
    assert.ok(child);
    return child;
  };
  return { service, children, current };
}

test("reuses a background helper and correlates permission/show replies out of order", async () => {
  const { service, children, current } = fixture();
  const permission = service.permission();
  const notification = {
    id: "csgclaw-test",
    title: "完成",
    body: "报告已完成",
    roomID: "room-1",
    silent: true,
  };
  const shown = service.show(notification);
  assert.equal(children.length, 1);
  assert.deepEqual(current().requests[1]?.notification, notification);
  current().reply(1, { ok: true });
  current().reply(0, { ok: true, permission: "granted" });
  await shown;
  assert.equal(await permission, "granted");
  service.stop();
});

test("routes a click only to its notification and unregisters closed handles", async () => {
  const { service, current } = fixture();
  let first = 0;
  let second = 0;
  const closeFirst = service.watch("one", () => first++);
  service.watch("two", () => second++);
  const permission = service.permission();
  current().reply(0, { ok: true, permission: "granted" });
  current().stdout.write(
    `${JSON.stringify({ event: "click", notification_id: "one" })}\n`,
  );
  assert.equal(first, 1);
  assert.equal(second, 0);
  closeFirst();
  current().stdout.write(
    `${JSON.stringify({ event: "click", notification_id: "one" })}\n`,
  );
  assert.equal(first, 1);
  service.stop();
  await permission;
});

test("rejects pending delivery on a crash and starts a fresh helper for the next request", async () => {
  const { service, children, current } = fixture();
  const pending = service.permission();
  current().emit("exit", 1);
  await assert.rejects(pending, /exited/);
  const next = service.permission();
  assert.equal(children.length, 2);
  current().reply(0, { ok: true, permission: "denied" });
  assert.equal(await next, "denied");
  service.stop();
});

test("bounds incomplete frames and rejects native errors without treating them as delivery", async () => {
  const { service, current } = fixture();
  const denied = service.permission(true);
  current().reply(0, { ok: false, error: "notification_permission_required" });
  await assert.rejects(denied, /notification_permission_required/);
  const corrupt = service.permission();
  current().stdout.write("x".repeat(65537));
  await assert.rejects(corrupt, /exceeded/);
  assert.equal(current().killed, true);
  service.stop();
});
