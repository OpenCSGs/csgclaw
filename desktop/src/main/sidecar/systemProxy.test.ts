import assert from "node:assert/strict";
import test from "node:test";
import {
  CLIPROXY_SYSTEM_PROXY_ENV,
  firstProxyURL,
  resolveSystemCLIProxyEnvironment,
} from "./systemProxy";

test("scopes the Electron system proxy to embedded CLIProxy", async () => {
  const env = await resolveSystemCLIProxyEnvironment(async () => "PROXY 127.0.0.1:7890");

  assert.deepEqual(env, {
    [CLIPROXY_SYSTEM_PROXY_ENV]: "http://127.0.0.1:7890",
  });
});

test("keeps direct system proxy rules unset", async () => {
  assert.deepEqual(await resolveSystemCLIProxyEnvironment(async () => "DIRECT"), {});
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
