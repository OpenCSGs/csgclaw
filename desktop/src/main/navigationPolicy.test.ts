import assert from "node:assert/strict";
import { test } from "node:test";
import { isSafeHTTPSURL, isSafeWebURL } from "./navigationPolicy";

test("does not hand commands or custom protocols to the operating system", () => {
  for (const target of [
    "csgclaw-cli",
    "csgclaw-cli:",
    "csgclaw-cli://run",
    "file:///C:/Windows/System32/cmd.exe",
  ]) {
    assert.equal(isSafeHTTPSURL(target), false, target);
  }
});

test("only accepts credential-free HTTPS links", () => {
  assert.equal(isSafeHTTPSURL("https://opencsg.com/docs"), true);
  assert.equal(isSafeHTTPSURL("http://opencsg.com/docs"), false);
  assert.equal(isSafeHTTPSURL("https://user:password@opencsg.com/docs"), false);
});

test("browser external navigation accepts HTTP and HTTPS but no privileged protocols", () => {
  assert.equal(isSafeWebURL("http://127.0.0.1:3000/docs"), true);
  assert.equal(isSafeWebURL("https://example.org/"), true);
  for (const target of ["javascript:alert(1)", "file:///etc/passwd", "csgclaw-cli://run", "https://user:password@example.org/"]) {
    assert.equal(isSafeWebURL(target), false, target);
  }
});
