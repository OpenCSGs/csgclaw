import { cp, lstat, mkdir, mkdtemp, readdir, realpath, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const output = fileURLToPath(new URL("../web/custom/static-dist", import.meta.url));

// Copy only a completed frontend build. Never modify its source directory.
export async function prepareCustomUI(source, destination = output) {
  if (!source) throw new Error("UI_DIR must point to a frontend build containing index.html");
  source = await realpath(source);
  destination = path.resolve(destination);
  // macOS aliases /var and /tmp through /private; compare canonical parent paths.
  destination = path.join(await realpath(path.dirname(destination)), path.basename(destination));
  if (source === destination || destination.startsWith(source + path.sep) || source.startsWith(destination + path.sep)) {
    throw new Error("UI_DIR and the generated embed directory must not overlap");
  }
  if (!(await lstat(path.join(source, "index.html"))).isFile()) {
    throw new Error("UI_DIR/index.html must be a regular file");
  }
  const files = [];
  async function collect(directory, relative = "") {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      // Build outputs should not bundle source control, environment files or dependency trees.
      if (entry.name.startsWith(".") || entry.name === "node_modules") continue;
      const name = path.join(relative, entry.name);
      if (entry.isDirectory()) await collect(path.join(source, name), name);
      else if (entry.isFile()) files.push(name);
      else throw new Error(`UI build contains a symbolic link or special file: ${name}`);
    }
  }
  await collect(source);
  await mkdir(path.dirname(destination), { recursive: true });
  const staged = await mkdtemp(path.join(path.dirname(destination), ".prepare-"));
  try {
    for (const file of files) {
      await mkdir(path.dirname(path.join(staged, file)), { recursive: true });
      await cp(path.join(source, file), path.join(staged, file));
    }
    await writeFile(path.join(staged, ".gitkeep"), "");
    // destination contains generated resources only; replacing it removes stale assets.
    await rm(destination, { recursive: true, force: true });
    await rename(staged, destination);
  } finally {
    await rm(staged, { recursive: true, force: true });
  }
  return files.length;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 3) throw new Error("usage: node scripts/prepare-custom-ui.mjs <frontend-dist>");
    console.log(`Custom UI prepared: ${await prepareCustomUI(process.argv[2])} files`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
