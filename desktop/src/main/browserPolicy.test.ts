import assert from "node:assert/strict";
import test from "node:test";
import {
  browserURL,
  clipBrowserBounds,
  parseBrowserRequest,
} from "./browserPolicy";

test("remote browsing rejects application access, credentials and privileged schemes", () => {
  const origin = "http://127.0.0.1:18080";
  assert.equal(browserURL("https://qq.com", origin), "https://qq.com/");
  assert.equal(
    browserURL("http://localhost:3000", origin),
    "http://localhost:3000/",
  );
  for (const url of [
    origin + "/api/v1/bootstrap",
    "file:///etc/passwd",
    "javascript:alert(1)",
    "data:text/html,hi",
    "https://user:secret@example.org",
    "csgclaw-cli://run",
  ])
    assert.throws(() => browserURL(url, origin));
});
test("browser requests and bounds are validated at the IPC boundary", () => {
  const request = {
    id: "tab",
    action: "sync",
    url: "https://qq.com",
    visible: true,
    bounds: { x: 10, y: 20, width: 500, height: 400 },
  };
  assert.deepEqual(parseBrowserRequest(request), request);
  for (const value of [
    null,
    {},
    { ...request, action: "executeJavaScript" },
    { ...request, id: "" },
    { ...request, visible: 1 },
    { ...request, bounds: { ...request.bounds, x: NaN } },
    { ...request, bounds: { ...request.bounds, width: -1 } },
  ])
    assert.throws(() => parseBrowserRequest(value));
  assert.deepEqual(
    clipBrowserBounds({ x: -50, y: 20, width: 200, height: 600 }, 100, 400),
    { x: 0, y: 20, width: 100, height: 380 },
  );
  assert.deepEqual(
    clipBrowserBounds({ x: 300, y: 300, width: 100, height: 100 }, 200, 200),
    { x: 200, y: 200, width: 0, height: 0 },
  );
});
