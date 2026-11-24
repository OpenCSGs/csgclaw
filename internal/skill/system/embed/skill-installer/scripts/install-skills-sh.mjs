import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, delimiter, join } from 'node:path';
import { parseArgs } from 'node:util';

function download(repo, name, cwd) {
  const args = ['add', repo, '--skill', name, '--agent', 'codex', '--copy', '-y'];
  const options = { cwd, stdio: 'inherit', timeout: 120_000 };
  const dirs = (process.env.PATH || '').split(delimiter).filter(Boolean);
  if (process.platform === 'win32') {
    // npm shims are batch files. Run their JS entrypoint with argv, not cmd.exe.
    for (const dir of dirs) {
      const exe = join(dir, 'skills.exe');
      if (existsSync(exe)) return execFileSync(exe, args, options);
      if (existsSync(join(dir, 'skills.cmd'))) {
        const modules = basename(dir) === '.bin' ? join(dir, '..') : join(dir, 'node_modules');
        return execFileSync(process.execPath, [join(modules, 'skills', 'bin', 'cli.mjs'), ...args], options);
      }
    }
    const npx = dirs.map(dir => join(dir, 'node_modules', 'npm', 'bin', 'npx-cli.js')).find(path => existsSync(path));
    if (!npx) throw new Error('Neither skills nor the npm npx entrypoint was found on PATH');
    return execFileSync(process.execPath, [npx, '--yes', 'skills@1.9.0', ...args], options);
  }
  try {
    return execFileSync('skills', args, options);
  } catch (error) {
    // A present CLI with a missing interpreter, or a failed add, is not a miss.
    if (error.code !== 'ENOENT' || dirs.some(dir => existsSync(join(dir, 'skills')))) throw error;
  }
  return execFileSync('npx', ['--yes', 'skills@1.9.0', ...args], options);
}

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
  const cwd = mkdtempSync(join(tmpdir(), 'csgclaw-skills-'));
  try {
    download(repo, name, cwd);
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
