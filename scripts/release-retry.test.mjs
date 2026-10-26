import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// Exercise the actual github-script body without GitHub credentials or API writes.
const workflow = readFileSync(
  new URL("../.github/workflows/release-retry.yml", import.meta.url),
  "utf8",
);
const script = workflow.split("          script: |\n")[1];
assert.ok(script, "Release retry workflow must contain its github-script body");
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const retry = new AsyncFunction(
  "github",
  "context",
  "core",
  script.replace(/^ {12}/gm, ""),
);

async function execute({
  run: overrides = {},
  eventAttempt = 1,
  apiError,
} = {}) {
  const run = {
    id: 123,
    path: ".github/workflows/release.yml",
    event: "push",
    head_repository: { full_name: "OpenCSGs/csgclaw" },
    status: "completed",
    conclusion: "failure",
    run_attempt: 1,
    html_url: "https://github.com/OpenCSGs/csgclaw/actions/runs/123",
    ...overrides,
  };
  const calls = [];
  const warnings = [];
  const summaries = [];
  const summary = {
    addHeading(value) {
      summaries.push(value);
      return this;
    },
    addLink() {
      return this;
    },
    addRaw(value) {
      summaries.push(value);
      return this;
    },
    async write() {},
  };
  const github = {
    rest: {
      actions: {
        async getWorkflowRun(parameters) {
          assert.deepEqual(parameters, {
            owner: "OpenCSGs",
            repo: "csgclaw",
            run_id: 123,
          });
          return { data: run };
        },
        async reRunWorkflowFailedJobs(parameters) {
          if (apiError) throw apiError;
          calls.push(parameters);
        },
      },
    },
  };
  await retry(
    github,
    {
      repo: { owner: "OpenCSGs", repo: "csgclaw" },
      payload: { workflow_run: { id: 123, run_attempt: eventAttempt } },
    },
    {
      info() {},
      warning(message) {
        warnings.push(message);
      },
      summary,
    },
  );
  return { calls, warnings, summaries };
}

for (const attempt of [1, 2]) {
  test(`retries only failed jobs after attempt ${attempt}`, async () => {
    const result = await execute({
      run: { run_attempt: attempt },
      eventAttempt: attempt,
    });
    assert.deepEqual(result.calls, [
      { owner: "OpenCSGs", repo: "csgclaw", run_id: 123 },
    ]);
    assert.equal(result.warnings.length, 0);
    assert.match(
      result.summaries.join(""),
      new RegExp(`attempt ${attempt + 1}/3`),
    );
  });
}

test("also retries a failed manual alpha release", async () => {
  assert.equal(
    (await execute({ run: { event: "workflow_dispatch" } })).calls.length,
    1,
  );
});

for (const attempt of [3, 4]) {
  test(`stops and reports the retry limit at attempt ${attempt}`, async () => {
    const result = await execute({
      run: { run_attempt: attempt },
      eventAttempt: attempt,
    });
    assert.equal(result.calls.length, 0);
    assert.match(result.warnings[0], /automatic retries stopped/);
    assert.match(result.summaries.join(""), /limit reached/);
  });
}

for (const conclusion of ["success", "cancelled", "skipped", "neutral"]) {
  test(`does not retry a ${conclusion} run`, async () => {
    assert.equal((await execute({ run: { conclusion } })).calls.length, 0);
  });
}

for (const status of ["queued", "in_progress", "waiting"]) {
  test(`does not retry a run that is ${status}`, async () => {
    assert.equal((await execute({ run: { status } })).calls.length, 0);
  });
}

test("ignores a delayed event after a manual or duplicate retry", async () => {
  assert.equal(
    (await execute({ run: { run_attempt: 2 }, eventAttempt: 1 })).calls.length,
    0,
  );
});

test("does not retry a different workflow with the Release name", async () => {
  assert.equal(
    (await execute({ run: { path: ".github/workflows/other.yml" } })).calls
      .length,
    0,
  );
});

test("does not retry a pull request run", async () => {
  assert.equal(
    (await execute({ run: { event: "pull_request" } })).calls.length,
    0,
  );
});

test("does not retry runs originating in a fork or a deleted repository", async () => {
  for (const head_repository of [{ full_name: "someone/csgclaw" }, null]) {
    assert.equal((await execute({ run: { head_repository } })).calls.length, 0);
  }
});

test("surfaces a rejected retry request instead of reporting success", async () => {
  await assert.rejects(
    execute({ apiError: new Error("Actions API denied retry") }),
    /denied retry/,
  );
});
