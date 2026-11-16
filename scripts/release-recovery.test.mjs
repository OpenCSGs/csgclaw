import assert from "node:assert/strict";
import test from "node:test";
import { validateRecovery } from "./release-recovery.mjs";

function fixture() {
  const repository = "OpenCSGs/csgclaw";
  const version = "v0.11.0-beta.5";
  const run = {
    id: 123,
    head_repository: { full_name: repository },
    event: "push",
    path: ".github/workflows/release.yml",
    head_branch: version,
    status: "completed",
    conclusion: "failure",
    head_sha: "a".repeat(40),
  };
  const jobs = ["prepare", "web"].map((name) => ({
    name,
    conclusion: "success",
  }));
  const artifacts = [];
  for (const [os, arch] of [
    ["linux", "amd64"],
    ["linux", "arm64"],
    ["darwin", "amd64"],
    ["darwin", "arm64"],
    ["windows", "amd64"],
  ]) {
    jobs.push({ name: `build (${os}, ${arch})`, conclusion: "success" });
    const failed = os === "windows";
    jobs.push({
      name: `desktop-build (${os}, ${arch}, runner)`,
      conclusion: failed ? "failure" : "success",
    });
    for (const name of [
      `dist-${os}-${arch}`,
      ...(failed ? [] : [`desktop-dist-${os}-${arch}`]),
    ]) {
      artifacts.push({
        id: artifacts.length + 1,
        name,
        expired: false,
        size_in_bytes: 100,
        workflow_run: { head_sha: run.head_sha },
      });
    }
  }
  return { repository, version, run, jobs, artifacts, target: "windows-amd64" };
}

test("selects original archives and successful desktops without mixing in a failed desktop", () => {
  const input = fixture();
  input.artifacts.push({
    id: 99,
    name: "desktop-dist-windows-amd64",
    expired: false,
    size_in_bytes: 100,
  });
  const result = validateRecovery(input);
  assert.equal(result.source_ref, input.run.head_sha);
  assert.equal(result.recovery_run_id, "123");
  assert.equal(result.recovery_archive_ids.split(",").length, 5);
  assert.equal(result.recovery_desktop_ids.split(",").length, 4);
  assert.ok(!result.recovery_desktop_ids.split(",").includes("99"));
});

for (const [name, mutate] of [
  [
    "different version",
    (x) => {
      x.version = "v0.11.0-beta.6";
    },
  ],
  [
    "stable version",
    (x) => {
      x.version = "v0.11.0";
    },
  ],
  [
    "different repository",
    (x) => {
      x.run.head_repository.full_name = "other/repo";
    },
  ],
  [
    "manual run",
    (x) => {
      x.run.event = "workflow_dispatch";
    },
  ],
  [
    "different workflow",
    (x) => {
      x.run.path = ".github/workflows/other.yml";
    },
  ],
  [
    "active run",
    (x) => {
      x.run.status = "in_progress";
    },
  ],
  [
    "successful run",
    (x) => {
      x.run.conclusion = "success";
    },
  ],
  [
    "failed server build",
    (x) => {
      x.jobs.find((j) => j.name === "build (linux, amd64)").conclusion =
        "failure";
    },
  ],
  [
    "another failed desktop",
    (x) => {
      x.jobs.find((j) =>
        j.name.startsWith("desktop-build (darwin, amd64,"),
      ).conclusion = "failure";
    },
  ],
  [
    "already successful target",
    (x) => {
      x.jobs.find((j) =>
        j.name.startsWith("desktop-build (windows,"),
      ).conclusion = "success";
    },
  ],
  [
    "expired archive",
    (x) => {
      x.artifacts[0].expired = true;
    },
  ],
  [
    "missing desktop",
    (x) => {
      x.artifacts = x.artifacts.filter(
        (a) => a.name !== "desktop-dist-linux-arm64",
      );
    },
  ],
  [
    "wrong source SHA",
    (x) => {
      x.artifacts[0].workflow_run.head_sha = "b".repeat(40);
    },
  ],
]) {
  test(`rejects ${name}`, () => {
    const input = fixture();
    mutate(input);
    assert.throws(() => validateRecovery(input));
  });
}
