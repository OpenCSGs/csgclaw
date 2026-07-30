import assert from "node:assert/strict";
import test from "node:test";
import { firstProxyURL, resolveSystemProxyEnvironment } from "./systemProxy";

test("maps Electron system proxy rules to sidecar environment variables", async () => {
  const env = await resolveSystemProxyEnvironment(async () => "PROXY 127.0.0.1:7890");

  assert.deepEqual(env, {
    HTTP_PROXY: "http://127.0.0.1:7890",
    HTTPS_PROXY: "http://127.0.0.1:7890",
    NO_PROXY: "localhost,127.0.0.1,::1",
    http_proxy: "http://127.0.0.1:7890",
    https_proxy: "http://127.0.0.1:7890",
    no_proxy: "localhost,127.0.0.1,::1",
  });
});

test("keeps direct system proxy rules unset", async () => {
  assert.deepEqual(await resolveSystemProxyEnvironment(async () => "DIRECT"), {});
});

test("uses the first supported proxy rule", () => {
  assert.equal(
    firstProxyURL("INVALID; SOCKS5 127.0.0.1:7891; PROXY 127.0.0.1:7890; DIRECT"),
    "socks5://127.0.0.1:7891",
  );
});

test("ignores malformed and credential-bearing proxy rules", () => {
  assert.equal(firstProxyURL("PROXY missing-port; PROXY user:pass@127.0.0.1:7890"), undefined);
});
