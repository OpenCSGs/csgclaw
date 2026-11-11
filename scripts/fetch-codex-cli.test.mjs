import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { pathToFileURL } from 'node:url';
import test from 'node:test';

const source = resolve(import.meta.dirname);

for (const platform of ['darwin', 'windows']) {
  for (const corrupt of [false, true]) {
    test(`${platform}: pinned download ${corrupt ? 'rejects corruption and preserves binary' : 'installs verified artifact'}`, () => {
      const root = mkdtempSync(resolve(source, '../.cache/csgclaw-dev/codex-fetch-test-'));
      try {
        const scripts = join(root, 'scripts');
        const mirror = join(root, 'mirror');
        const release = join(mirror, 'v1.2.3');
        const output = join(root, 'bundle', 'bin');
        for (const dir of [scripts, release, output]) mkdirSync(dir, { recursive: true });
        for (const file of ['fetch-codex-cli.sh', 'codex-cli-platforms.txt']) {
          copyFileSync(join(source, file), join(scripts, file));
        }
        writeFileSync(join(scripts, 'codex-cli-version.txt'), '1.2.3\n');
        const isWindows = platform === 'windows';
        const executable = isWindows ? 'codex.exe' : 'codex';
        const artifact = `codex-package_1.2.3_${platform}_arm64.tar.gz`;
        const artifactPath = join(release, artifact);
        const content = Buffer.from('verified executable\n');
        const packageRoot = join(root, 'package');
        mkdirSync(join(packageRoot, 'bin'), { recursive: true });
        mkdirSync(join(packageRoot, 'codex-resources'));
        mkdirSync(join(packageRoot, 'codex-path'));
        writeFileSync(join(packageRoot, 'bin', executable), content);
        writeFileSync(join(packageRoot, 'bin', 'codex-exec-server'), 'helper');
        writeFileSync(join(packageRoot, 'codex-package.json'), '{"version":"1.2.3"}');
        writeFileSync(join(packageRoot, 'codex-resources', 'bwrap'), 'resource');
        writeFileSync(join(packageRoot, 'codex-path', 'rg'), 'search');
        execFileSync('tar', ['-czf', artifactPath, '-C', packageRoot, '.']);
        const digest = createHash('sha256').update(readFileSync(artifactPath)).digest('hex');
        writeFileSync(join(scripts, 'codex-cli-checksums.txt'), `${digest}  ${artifact}\n`);
        if (corrupt) writeFileSync(artifactPath, 'corrupted download');
        writeFileSync(join(output, executable), 'previous executable');
        const result = spawnSync('bash', [join(scripts, 'fetch-codex-cli.sh'), platform, 'arm64', output], {
          env: { ...process.env, CODEX_CLI_DOWNLOAD_BASE_URL: pathToFileURL(mirror).href },
          encoding: 'utf8',
        });
        assert.ok(result.stdout.includes(`/v1.2.3/${artifact}`), result.stdout);
        if (corrupt) {
          assert.notEqual(result.status, 0);
          assert.match(result.stderr, /checksum mismatch/);
          assert.equal(readFileSync(join(output, executable), 'utf8'), 'previous executable');
        } else {
          assert.equal(result.status, 0, result.stderr);
          assert.deepEqual(readFileSync(join(output, executable)), content);
          assert.equal(readFileSync(join(output, 'codex-exec-server'), 'utf8'), 'helper');
          assert.equal(readFileSync(join(output, '../codex-resources/bwrap'), 'utf8'), 'resource');
          assert.equal(readFileSync(join(output, '../codex-path/rg'), 'utf8'), 'search');
          assert.equal(JSON.parse(readFileSync(join(output, '../codex-package.json'))).version, '1.2.3');
        }
      } finally {
        rmSync(root, { recursive: true, force: true });
      }
    });
  }
}
