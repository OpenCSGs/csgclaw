import assert from "node:assert/strict";
import test from "node:test";
import { usesMicrosoftStoreUpdates } from "./updatePolicy";

test("Microsoft Store packages use Store-managed updates", () => {
  assert.equal(usesMicrosoftStoreUpdates("win32", true), true);
});

test("non-Store Windows packages keep the configured update feed", () => {
  assert.equal(usesMicrosoftStoreUpdates("win32", false), false);
  assert.equal(usesMicrosoftStoreUpdates("win32", undefined), false);
});

test("non-Windows packages do not use Microsoft Store updates", () => {
  assert.equal(usesMicrosoftStoreUpdates("darwin", true), false);
  assert.equal(usesMicrosoftStoreUpdates("linux", true), false);
});
