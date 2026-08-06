import assert from "node:assert/strict";
import test from "node:test";
import { DeferredRelaunch } from "./deferredRelaunch";

test("does not schedule a relaunch without an open request", () => {
  const deferred = new DeferredRelaunch();
  let schedules = 0;

  assert.equal(deferred.scheduleIfRequested(() => schedules++), false);
  assert.equal(schedules, 0);
});

test("coalesces repeated open requests into one relaunch", () => {
  const deferred = new DeferredRelaunch();
  let schedules = 0;

  assert.equal(deferred.request(), true);
  assert.equal(deferred.request(), false);
  assert.equal(deferred.scheduleIfRequested(() => schedules++), true);
  assert.equal(deferred.scheduleIfRequested(() => schedules++), false);
  assert.equal(schedules, 1);
});
