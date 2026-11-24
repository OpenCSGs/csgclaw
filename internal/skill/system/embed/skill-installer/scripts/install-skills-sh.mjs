import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { delimiter, join } from 'node:path';
import { parseArgs } from 'node:util';

function install() {
  if (/^(1|true|t)$/i.test(process.env.CSGCLAW_DISABLE_PUBLIC_SKILLS || '')) {
    throw new Error('Public skills.sh access is disabled; use the global library or your signed-in site.');
  }
  const { values, positionals } = parseArgs({
    allowPositionals: true,
    options: { skill: { type: 'string' }, force: { type: 'boolean' }, confirmation: { type: 'string' } },
  });
  const pattern = /^[a-zA-Z0-9][a-zA-Z0-9._-]*$/;
  const source = positionals[0] || '';
  const page = /^https:\/\/skills\.sh\/([^/]+)\/([^/]+)\/([^/?#]+)\/?$/.exec(source);
  const repo = page ? `${page[1]}/${page[2]}` : source.replace(/^https:\/\/github\.com\//, '').replace(/\/$/, '');
  const name = values.skill || page?.[3];
  if (positionals.length !== 1 || repo.split('/').length !== 2 ||
      !repo.split('/').every(part => pattern.test(part)) || !name || !pattern.test(name) ||
      (page && page[3] !== name)) {
    throw new Error('Usage: install-skills-sh.mjs <skills.sh/owner/repo/skill URL | owner/repo --skill name> [--force]');
  }
  if (!process.env.CSGCLAW_CALLER_AGENT_ID || !(process.env.CODEX_HOME || process.env.DSH_HOME)) {
    throw new Error('Run this installer inside the target CSGClaw worker.');
  }
  let cmd = 'npx';
  const args = ['--yes', 'skills@1.9.0', 'add', repo, '--skill', name, '--agent', 'codex', '--copy', '-y'];
  if (process.platform === 'win32') {
    // Run npm's JS entrypoint rather than interpreting user arguments through cmd.exe.
    const entry = (process.env.PATH || '').split(delimiter)
      .map(dir => join(dir, 'node_modules', 'npm', 'bin', 'npx-cli.js')).find(path => existsSync(path));
    if (!entry) throw new Error('npx-cli.js not found in the Node.js installation on PATH');
    cmd = process.execPath;
    args.unshift(entry);
  }
  const cwd = mkdtempSync(join(tmpdir(), 'csgclaw-skills-'));
  try {
    execFileSync(cmd, args, { cwd, stdio: 'inherit', timeout: 120_000 });
    const cli = process.env.CSGCLAW_CLI || 'csgclaw-cli';
    const root = join(cwd, '.agents', 'skills');
    const installed = readdirSync(root, { withFileTypes: true }).filter(entry => entry.isDirectory());
    if (installed.length !== 1 || !pattern.test(installed[0].name)) {
      throw new Error('Expected exactly one installed skill in the temporary project');
    }
    const installedName = installed[0].name;
    const importArgs = ['--output', 'json', 'skill', 'import', join(root, installedName), '--source-url', `https://skills.sh/${repo}/${name}`];
    if (values.force) importArgs.push('--force');
    if (values.confirmation) importArgs.push('--confirmation', values.confirmation);
    execFileSync(cli, importArgs, { stdio: 'inherit' });
  } finally {
    rmSync(cwd, { recursive: true, force: true });
  }
}

try {
  install();
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
