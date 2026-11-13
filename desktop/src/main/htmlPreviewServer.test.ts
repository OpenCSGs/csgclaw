import assert from "node:assert/strict";
import test from "node:test";
import { request } from "node:http";
import { HTMLPreviewServer } from "./htmlPreviewServer";

test("HTML previews serve executable documents without application credentials", async () => {
  const previews = new HTMLPreviewServer();
  try {
    const html = '<!doctype html><canvas></canvas><script>localStorage.setItem("score", "1")</script>';
    const url = await previews.publish("tab", html, "file-a");
    const response = await fetch(url);
    assert.equal(response.status, 200);
    assert.equal(await response.text(), html);
    assert.equal(response.headers.get("content-type"), "text/html; charset=utf-8");
    assert.equal(response.headers.get("cache-control"), "no-store");
    assert.equal(response.headers.get("set-cookie"), null);
    assert.equal(response.headers.get("content-disposition"), null);
    assert.equal((await fetch(new URL("/api/v1/bootstrap", url))).status, 404);
    assert.equal((await fetch(new URL("/", url))).status, 404);
    assert.equal((await fetch(url, { method: "POST", body: "ignored" })).status, 404);
    const status = await new Promise<number | undefined>((resolve, reject) => {
      const req = request(url, { headers: { Host: "attacker.example" } }, (res) => {
        res.resume();
        resolve(res.statusCode);
      });
      req.on("error", reject);
      req.end();
    });
    assert.equal(status, 404);
    const replacement = await previews.publish("tab", "<h1>Updated</h1>", "file-a");
    assert.equal((await fetch(url)).status, 404);
    assert.equal(await (await fetch(replacement)).text(), "<h1>Updated</h1>");
    previews.release("tab");
    assert.equal((await fetch(replacement)).status, 404);
  } finally {
    previews.dispose();
    await assert.rejects(previews.publish("closed", "unavailable", "file-a"));
  }
});

test("concurrent HTML previews share one listener and release independently", async () => {
  const previews = new HTMLPreviewServer();
  try {
    const [first, second] = await Promise.all([
      previews.publish("first", "first", "file-a"),
      previews.publish("second", "second", "file-b"),
    ]);
    assert.equal(new URL(first).origin, new URL(second).origin);
    assert.notEqual(first, second);
    assert.equal(previews.sourceForURL(first), "file-a");
    assert.equal(previews.sourceForURL(first + "#level-2"), "file-a");
    assert.equal(previews.sourceForURL(second), "file-b");
    assert.equal(previews.allowsRequest(first, "file-a"), true);
    assert.equal(previews.allowsRequest(first, "file-b"), false);
    assert.equal(previews.allowsRequest(first), false);
    assert.equal(previews.allowsRequest(second, "file-a"), false);
    assert.equal(previews.allowsRequest(new URL("/", first).href, "file-a"), false);
    assert.equal(previews.allowsRequest("https://example.org/", "file-a"), true);
    assert.equal(previews.allowsRequest(`blob:${new URL(first).origin}/worker`, "file-a"), true);
    previews.release("first");
    assert.equal(previews.sourceForURL(first), undefined);
    assert.equal(previews.allowsRequest(first, "file-a"), false);
    assert.equal((await fetch(first)).status, 404);
    assert.equal(await (await fetch(second)).text(), "second");
    previews.clear();
    assert.equal((await fetch(second)).status, 404);
  } finally {
    previews.dispose();
    await assert.rejects(previews.publish("closed", "unavailable", "file-a"));
  }
});
