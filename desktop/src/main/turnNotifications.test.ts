import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import test from "node:test";
import type { Notification } from "electron";
import { TurnNotifications, parseTurnNotification } from "./turnNotifications";

const input = {
  id: "turn-test",
  silent: false,
  title: "Agent finished",
  body: "Final reply",
  roomID: "room-test",
};

class FakeNotification extends EventEmitter {
  closed = false;
  show(): void {
    this.emit("show");
  }
  close(): void {
    this.closed = true;
    this.emit("close");
  }
}

test("native notifications keep their final text, deduplicate and route clicks", async () => {
  const created: FakeNotification[] = [];
  const clicked: string[] = [];
  const notifications = new TurnNotifications(
    (room) => clicked.push(room),
    (options) => {
      assert.deepEqual(options, input);
      const notification = new FakeNotification();
      created.push(notification);
      return notification as unknown as Notification;
    },
    () => true,
  );
  await notifications.show(input);
  await notifications.show(input);
  assert.equal(created.length, 1);
  assert.ok(created[0]);
  created[0].emit("click");
  assert.deepEqual(clicked, ["room-test"]);
  assert.equal(created[0].closed, true);
  notifications.dispose();
});

test("native delivery failures reject and allow retry", async () => {
  let attempts = 0;
  const notifications = new TurnNotifications(
    () => {},
    () => {
      const notification = new FakeNotification();
      notification.show = () => {
        attempts++;
        if (attempts === 1)
          notification.emit("failed", {}, "permission denied");
        else notification.emit("show");
      };
      return notification as unknown as Notification;
    },
    () => true,
  );
  await assert.rejects(notifications.show(input), /permission denied/);
  await notifications.show(input);
  assert.equal(attempts, 2);
  notifications.dispose();
});

test("IPC input rejects invalid and unbounded payloads", () => {
  assert.deepEqual(parseTurnNotification(input), input);
  for (const invalid of [
    null,
    [],
    { ...input, title: "" },
    { ...input, id: "x".repeat(65) },
    { ...input, roomID: 12 },
    { ...input, silent: "false" },
  ]) {
    assert.throws(() => parseTurnNotification(invalid));
  }
});

test("silent notifications preserve the banner title and body", async () => {
  const silent = { ...input, silent: true };
  const notifications = new TurnNotifications(
    () => {},
    (options) => {
      assert.deepEqual(options, silent);
      return new FakeNotification() as unknown as Notification;
    },
    () => true,
  );
  await notifications.show(silent);
  notifications.dispose();
});
