import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const platforms = readFileSync(
  new URL("./release-platforms.txt", import.meta.url),
  "utf8",
)
  .split(/\r?\n/)
  .map((line) => line.trim())
  .filter((line) => line && !line.startsWith("#"))
  .map((line) => line.split(/\s+/));

export function validateRecovery({
  run,
  jobs,
  artifacts,
  repository,
  version,
  target,
}) {
  if (
    !/^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-beta\.(0|[1-9]\d*)$/.test(
      version,
    )
  ) {
    throw new Error(
      "Recovery requires an existing vMAJOR.MINOR.PATCH-beta.NUMBER tag",
    );
  }
  if (
    run.head_repository?.full_name !== repository ||
    run.event !== "push" ||
    run.path !== ".github/workflows/release.yml" ||
    run.head_branch !== version ||
    run.status !== "completed" ||
    run.conclusion !== "failure" ||
    !/^[0-9a-f]{40}$/.test(run.head_sha) ||
    !Number.isSafeInteger(run.id) ||
    run.id <= 0
  ) {
    throw new Error(
      "Source must be a failed tag-triggered Release run in this repository",
    );
  }
  if (!platforms.some(([os, arch]) => `${os}-${arch}` === target)) {
    throw new Error("Unsupported recovery target");
  }
  const requireJob = (name, prefix = false) => {
    const matches = jobs.filter((job) =>
      prefix ? job.name.startsWith(name) : job.name === name,
    );
    if (matches.length !== 1)
      throw new Error(`Missing or ambiguous source job: ${name}`);
    return matches[0];
  };
  for (const name of ["prepare", "web"]) {
    if (requireJob(name).conclusion !== "success")
      throw new Error(`Source job failed: ${name}`);
  }
  const archiveIds = [];
  const desktopIds = [];
  const requireArtifact = (name) => {
    const matches = artifacts.filter((artifact) => artifact.name === name);
    if (
      matches.length !== 1 ||
      matches[0].expired ||
      !(matches[0].size_in_bytes > 0) ||
      !Number.isSafeInteger(matches[0].id) ||
      matches[0].id <= 0 ||
      (matches[0].workflow_run?.head_sha &&
        matches[0].workflow_run.head_sha !== run.head_sha)
    ) {
      throw new Error(`Missing, expired, or invalid source artifact: ${name}`);
    }
    return matches[0].id;
  };
  for (const [os, arch] of platforms) {
    const platform = `${os}-${arch}`;
    if (requireJob(`build (${os}, ${arch})`).conclusion !== "success") {
      throw new Error(`Server/CLI build did not succeed: ${platform}`);
    }
    archiveIds.push(requireArtifact(`dist-${platform}`));
    const desktop = requireJob(`desktop-build (${os}, ${arch}, `, true);
    if (platform === target) {
      if (desktop.conclusion !== "failure")
        throw new Error("Selected desktop build did not fail");
    } else {
      if (desktop.conclusion !== "success")
        throw new Error(`Another desktop target also failed: ${platform}`);
      desktopIds.push(requireArtifact(`desktop-dist-${platform}`));
    }
  }
  return {
    recovery_run_id: String(run.id),
    source_ref: run.head_sha,
    recovery_archive_ids: archiveIds.join(","),
    recovery_desktop_ids: desktopIds.join(","),
  };
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const [runPath, jobsPath, artifactsPath, version, target] =
    process.argv.slice(2);
  const read = (path) => JSON.parse(readFileSync(path, "utf8"));
  const result = validateRecovery({
    run: read(runPath),
    jobs: read(jobsPath).jobs,
    artifacts: read(artifactsPath).artifacts,
    repository: process.env.GITHUB_REPOSITORY,
    version,
    target,
  });
  for (const [key, value] of Object.entries(result))
    console.log(`${key}=${value}`);
}
