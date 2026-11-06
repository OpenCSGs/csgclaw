import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, writeFile, symlink, rm, stat } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { prepareCustomUI } from "./prepare-custom-ui.mjs";

test("embeds only build assets, removes stale output, and leaves invalid inputs untouched", async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "csgclaw-ui-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = path.join(root, "source"), output = path.join(root, "output");
  await mkdir(source);
  await mkdir(output);
  await writeFile(path.join(output, "old.js"), "old");
  await assert.rejects(prepareCustomUI(source, output));
  assert.equal(await readFile(path.join(output, "old.js"), "utf8"), "old");
  await writeFile(path.join(source, "index.html"), "custom");
  await writeFile(path.join(source, ".env"), "excluded test fixture");
  assert.equal(await prepareCustomUI(source, output), 1);
  assert.equal(await readFile(path.join(output, "index.html"), "utf8"), "custom");
  await assert.rejects(stat(path.join(output, "old.js")));
  await assert.rejects(stat(path.join(output, ".env")));
  await assert.rejects(prepareCustomUI(source, path.join(source, "output")), /overlap/);
  await symlink(path.join(output, "index.html"), path.join(source, "outside.html"));
  await assert.rejects(prepareCustomUI(source, output), /symbolic link/);
  assert.equal(await readFile(path.join(output, "index.html"), "utf8"), "custom");
});
