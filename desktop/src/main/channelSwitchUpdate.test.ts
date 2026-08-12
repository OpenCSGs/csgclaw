import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import crypto from "node:crypto";
import {
  downloadVerifiedArtifact,
  isVerifiedArtifact,
  officialMacReleaseArchiveURL,
  parseMacChannelUpdate,
  startMacChannelUpdateFeed,
} from "./channelSwitchUpdate";

test("selects the target latest release from a static macOS feed", () => {
  assert.deepEqual(
    parseMacChannelUpdate(
      {
        currentRelease: "0.4.6",
        releases: [
          {
            version: "0.4.6",
            updateTo: {
              name: "CSGClaw v0.4.6",
              url: "https://downloads.example/CSGClaw-0.4.6.zip",
            },
          },
        ],
      },
      "0.4.6",
    ),
    {
      name: "CSGClaw v0.4.6",
      url: "https://downloads.example/CSGClaw-0.4.6.zip",
    },
  );
  assert.throws(
    () =>
      parseMacChannelUpdate({ currentRelease: "0.5.0", releases: [] }, "0.4.6"),
    /expected 0.4.6/,
  );
});

test("builds the official release archive fallback for macOS", () => {
  assert.equal(
    officialMacReleaseArchiveURL("0.4.6", "arm64"),
    "https://github.com/OpenCSGs/csgclaw/releases/download/v0.4.6/csgclaw-desktop_v0.4.6_darwin_arm64.zip",
  );
  assert.equal(
    officialMacReleaseArchiveURL("0.4.6", "x64"),
    "https://github.com/OpenCSGs/csgclaw/releases/download/v0.4.6/csgclaw-desktop_v0.4.6_darwin_amd64.zip",
  );
});

test("serves a one-use local macOS feed that can force the target version", async (context) => {
  let feed;
  try {
    feed = await startMacChannelUpdateFeed({
      name: "CSGClaw v0.4.6",
      url: "https://downloads.example/CSGClaw-0.4.6.zip",
    });
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "EPERM") {
      context.skip("The test sandbox does not allow binding a loopback port.");
      return;
    }
    throw error;
  }
  const response = await fetch(feed.url);
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), {
    name: "CSGClaw v0.4.6",
    url: "https://downloads.example/CSGClaw-0.4.6.zip",
  });
  await feed.close();
});

test("downloads and verifies a complete channel installer", async () => {
  const directory = await fs.promises.mkdtemp(
    path.join(os.tmpdir(), "csgclaw-channel-update-"),
  );
  const destination = path.join(directory, "update.exe");
  const payload = Buffer.from("signed installer fixture");
  const artifact = {
    url: "https://downloads.example/update.exe",
    sizeBytes: payload.length,
    sha256: crypto.createHash("sha256").update(payload).digest("hex"),
  };
  try {
    await downloadVerifiedArtifact(
      async () => new Response(payload),
      artifact,
      destination,
    );
    assert.deepEqual(await fs.promises.readFile(destination), payload);
    assert.equal(await isVerifiedArtifact(destination, artifact), true);
    assert.equal(
      await isVerifiedArtifact(destination, {
        ...artifact,
        sha256: "0".repeat(64),
      }),
      false,
    );
  } finally {
    await fs.promises.rm(directory, { recursive: true, force: true });
  }
});
