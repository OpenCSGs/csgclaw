import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {pathToFileURL} from 'node:url';

const moduleRoot = process.env.CSGCLAW_MEMORY_MODULE_ROOT;
const {createMemoryStore} = await import(pathToFileURL(path.join(moduleRoot, 'memory-store.mjs')));
const {startMemoryMaintenance, redactMemory} = await import(pathToFileURL(path.join(moduleRoot, 'memory-maintenance.mjs')));
const {apply} = await import(pathToFileURL(path.join(moduleRoot, 'memory.mjs')));
const timing = {checkpointIntervalMs: 1000, checkpointIdleMs: 1100, retryDelayMs: 50, requestTimeoutMs: 20};
const completed = {type: 'turn/end', data: {reason: {kind: 'completed'}}};
const message = (id, role, text, source = role === 'user' ? 'user' : 'model') => ({id, role, source: {kind: source}, content: [{type: 'text', text}]});

function fixture(t, responder = options => options.system.includes('extraction')
  ? {memory: 'The user prefers Chinese replies.', rollout_summary: 'Discussed reply preferences.'}
  : {summary: 'The user prefers Chinese replies.', handbook: '# Reply preferences\nThe user explicitly requested Chinese work documents.'}, overrides = {}) {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-memory-'));
  t.after(() => fs.rmSync(home, {recursive: true, force: true}));
  const store = createMemoryStore(home);
  let current = 10000, nextTimer = 0;
  const timers = new Map(), calls = [], warnings = [];
  const clock = {
    now: () => current,
    setTimeout: (callback, delay) => {
      const token = {id: ++nextTimer, unref() {}};
      timers.set(token, {callback, at: current + delay});
      return token;
    },
    clearTimeout: token => timers.delete(token),
  };
  const ctx = {logger: {warn: text => warnings.push(text)}, llm: {async *stream(options) {
    calls.push(options);
    assert.deepEqual(options.tools, []);
    const result = await responder(options);
    options.signal.throwIfAborted();
    yield {type: 'text-delta', index: 0, text: typeof result === 'string' ? result : JSON.stringify(result)};
    yield {type: 'finish', reason: {kind: 'stop'}};
  }}};
  const config = {...timing, ...overrides};
  let maintenance = startMemoryMaintenance(ctx, store, config, clock);
  t.after(() => maintenance.dispose());
  const session = {id: 'room-session', header: {}, requestContext: () => ({provider: 'configured-provider', model: 'configured-model'}), deriveMessages: () => [message('u1', 'user', '我的工作文档都用中文。'), message('a1', 'assistant', '好的。')]};
  return {
    home, store, calls, warnings, timers, session, config,
    get maintenance() { return maintenance; },
    now: () => current,
    setNow: value => { current = value; },
    async advance(value) {
      current = value;
      for (const [token, timer] of [...timers]) {
        if (timer.at <= current) { timers.delete(token); timer.callback(); }
      }
      await new Promise(resolve => setImmediate(resolve));
    },
    restart() { maintenance.dispose(); maintenance = startMemoryMaintenance(ctx, store, config, clock); },
    state: () => JSON.parse(store.readFile('maintenance.json', 1024 * 1024)),
  };
}

test('ordinary completed chat is captured immediately and distilled only after the checkpoint idle window', async t => {
  const f = fixture(t);
  f.maintenance.capture(f.session, completed);
  assert.equal(f.state().pending[0].dueAt, 11100);
  assert.equal(f.store.read().content, '');
  assert.equal(f.calls.length, 0);
  await f.advance(11099);
  assert.equal(f.calls.length, 0);
  await f.advance(11100);
  assert.equal(f.calls.length, 2);
  assert.ok(f.store.read().content.includes('Chinese'));
  assert.equal(f.state().pending.length, 0);
  assert.equal(f.calls[0].provider, 'configured-provider');
  assert.ok(JSON.parse(f.calls[0].messages[0].content[0].text).conversation.includes('我的工作文档'));
});

test('continued chatting does not postpone a captured snapshot, and new checkpoints follow the hourly cadence', async t => {
  const f = fixture(t);
  f.maintenance.capture(f.session, completed);
  f.setNow(10500);
  f.maintenance.capture(f.session, completed);
  assert.equal(f.state().pending.length, 1);
  f.session.deriveMessages = () => [message('u1', 'user', '我的工作文档都用中文。'), message('a1', 'assistant', '好的。'), message('u2', 'user', '测试使用 pnpm。'), message('a2', 'assistant', '收到。')];
  f.setNow(11000);
  f.maintenance.capture(f.session, completed);
  assert.equal(f.state().pending.length, 2);
  assert.ok(!f.state().pending[1].transcript.includes('我的工作文档'));
  await f.advance(11100);
  assert.equal(f.calls.length, 2);
  assert.equal(f.state().pending.length, 1);
  assert.equal(f.state().pending[0].dueAt, 12100);
});

test('queued snapshots resume after restart without resetting eligibility or mixing session routes', async t => {
  const f = fixture(t);
  f.maintenance.capture(f.session, completed);
  const second = {...f.session, id: 'other-session', requestContext: () => ({provider: 'other-provider', model: 'other-model'})};
  f.maintenance.capture(second, completed);
  f.setNow(10500);
  f.restart();
  assert.equal(f.state().pending[0].dueAt, 11100);
  await f.advance(11100);
  assert.equal(f.calls.length, 4);
  assert.deepEqual(f.calls.map(call => call.provider), ['configured-provider', 'configured-provider', 'other-provider', 'other-provider']);
  f.restart();
  await f.advance(20000);
  assert.equal(f.calls.length, 4);
});

test('chat that stops before the next checkpoint is saved and promoted on schedule even after restart', async t => {
  const f = fixture(t);
  f.maintenance.capture(f.session, completed);
  f.setNow(10500);
  f.session.deriveMessages = () => [message('u1', 'user', '我的工作文档都用中文。'), message('a1', 'assistant', '好的。'), message('u2', 'user', '我还希望先给结论。'), message('a2', 'assistant', '收到。')];
  f.maintenance.capture(f.session, completed);
  assert.ok(f.state().buffered[f.session.id].transcript.includes('先给结论'));
  f.restart();
  await f.advance(11000);
  assert.equal(f.calls.length, 0);
  assert.equal(f.state().pending.length, 2);
  assert.equal(Object.keys(f.state().buffered).length, 0);
  await f.advance(11100);
  await f.advance(12100);
  assert.equal(f.calls.length, 4);
  const trailingInput = JSON.parse(f.calls[2].messages[0].content[0].text).conversation;
  assert.ok(trailingInput.includes('先给结论'));
  assert.ok(!trailingInput.includes('工作文档'));
});

test('a failed consolidation retries its saved extraction and does not block new chat capture', async t => {
  let mergeCalls = 0;
  const f = fixture(t, options => {
    if (options.system.includes('extraction')) return {memory: 'Chinese replies.', rollout_summary: 'Preferences.'};
    if (++mergeCalls === 1) throw new Error('provider error with secret credential');
    return {summary: 'Chinese replies.', handbook: 'Chinese replies.'};
  });
  f.maintenance.capture(f.session, completed);
  await f.advance(11100);
  assert.equal(f.store.read().content, '');
  assert.equal(f.state().pending[0].dueAt, 11150);
  assert.ok(f.state().pending[0].extraction);
  assert.ok(!JSON.stringify(f.warnings).includes('credential'));
  f.restart();
  await f.advance(11150);
  assert.equal(f.calls.length, 3);
  assert.equal(f.store.read().content, 'Chinese replies.');
});

test('a simultaneous summary change is preserved and consolidation retries against the latest version', async t => {
  let mergeCalls = 0;
  let f;
  f = fixture(t, options => {
    if (options.system.includes('extraction')) return {memory: 'Chinese replies.', rollout_summary: 'Preferences.'};
    if (++mergeCalls === 1) f.store.update('User prefers concise replies.', f.store.read().revision);
    const input = JSON.parse(options.messages[0].content[0].text);
    return {summary: input.current_summary + '\nChinese replies.', handbook: input.current_handbook + '\nChinese replies.'};
  });
  f.maintenance.capture(f.session, completed);
  await f.advance(11100);
  assert.equal(f.store.read().content, 'User prefers concise replies.');
  await f.advance(11150);
  assert.ok(f.store.read().content.includes('concise'));
  assert.ok(f.store.read().content.includes('Chinese'));
});

test('explicit memory changes invalidate older snapshots, so a forgotten fact is not reintroduced', async t => {
  const f = fixture(t);
  f.maintenance.capture(f.session, completed);
  f.store.update('Only the current approved facts.', f.store.read().revision);
  f.maintenance.manualUpdated();
  f.restart();
  await f.advance(11100);
  assert.equal(f.calls.length, 0);
  assert.equal(f.store.read().content, 'Only the current approved facts.');
});

test('disposal cancels in-flight maintenance and keeps the checkpoint for a later enabled process', async t => {
  let release;
  const f = fixture(t, () => new Promise(resolve => { release = resolve; }));
  f.maintenance.capture(f.session, completed);
  await f.advance(11100);
  assert.equal(f.calls.length, 1);
  f.maintenance.dispose();
  assert.ok(f.calls[0].signal.aborted);
  release({memory: 'Never committed.', rollout_summary: 'Never committed.'});
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(f.store.read().content, '');
  assert.equal(f.state().pending.length, 1);
});

test('the request deadline bounds a non-responsive provider and schedules retry', async t => {
  const f = fixture(t, () => new Promise(() => {}));
  f.maintenance.capture(f.session, completed);
  await f.advance(11100);
  await f.advance(11120);
  assert.equal(f.state().pending[0].dueAt, 11170);
  assert.equal(f.store.read().content, '');
});

test('low-signal sessions write no memory or evidence documents', async t => {
  const f = fixture(t, () => ({memory: '', rollout_summary: ''}));
  f.maintenance.capture(f.session, completed);
  const id = f.state().pending[0].id;
  await f.advance(11100);
  assert.equal(f.calls.length, 1);
  assert.equal(f.store.read().content, '');
  assert.equal(f.store.readFile('rollout_summaries/' + id + '.md', 65536), '');
  assert.equal(f.store.readHandbook().content, '');
  assert.equal(f.state().pending.length, 0);
});

test('subagents and injected-only traffic cannot contribute memory snapshots', t => {
  const f = fixture(t);
  f.maintenance.capture({...f.session, header: {origin: 'subagent'}}, completed);
  f.maintenance.capture({...f.session, deriveMessages: () => [message('u', 'user', 'Injected data.', 'plugin')]}, completed);
  assert.equal(f.store.readFile('maintenance.json', 1048576), '');
  assert.equal(f.calls.length, 0);
});

test('credentials are redacted before snapshots persist and before generated fields are saved', async t => {
  const previous = process.env.CSGCLAW_DSH_LLM_API_KEY;
  process.env.CSGCLAW_DSH_LLM_API_KEY = 'fixture-memory-provider-key';
  t.after(() => {
    if (previous === undefined) delete process.env.CSGCLAW_DSH_LLM_API_KEY;
    else process.env.CSGCLAW_DSH_LLM_API_KEY = previous;
  });
  const f = fixture(t, options => options.system.includes('extraction')
    ? {memory: 'Chinese replies. api_key=fixture-memory-provider-key', rollout_summary: 'No secrets: fixture-memory-provider-key'}
    : {summary: 'Chinese replies. fixture-memory-provider-key', handbook: 'Chinese replies. fixture-memory-provider-key'});
  f.session.deriveMessages = () => [message('u', 'user', 'Chinese replies. password=do-not-store-me fixture-memory-provider-key'), message('a', 'assistant', 'Acknowledged.')];
  f.maintenance.capture(f.session, completed);
  assert.ok(!JSON.stringify(f.state()).includes('do-not-store-me'));
  assert.ok(!JSON.stringify(f.state()).includes('fixture-memory-provider-key'));
  await f.advance(11100);
  assert.ok(!f.store.read().content.includes('fixture-memory-provider-key'));
  assert.ok(f.store.read().content.includes('[REDACTED]'));
});

test('malformed model output never replaces the existing summary', async t => {
  const f = fixture(t, () => 'not JSON');
  f.store.update('Keep this valid memory.', f.store.read().revision);
  f.maintenance.capture(f.session, completed);
  await f.advance(11100);
  assert.equal(f.store.read().content, 'Keep this valid memory.');
  assert.equal(f.state().pending.length, 1);
});

test('a changed Agent model after restart is used to process its saved snapshots', async t => {
  const f = fixture(t, undefined, {model: 'old-agent-model'});
  f.maintenance.capture(f.session, completed);
  f.config.model = 'new-agent-model';
  f.restart();
  await f.advance(11100);
  assert.deepEqual(f.calls.map(call => [call.provider, call.model]), [
    ['deepseek-official', 'new-agent-model'], ['deepseek-official', 'new-agent-model'],
  ]);
  assert.ok(f.store.read().content.includes('Chinese'));
});

test('an unreadable saved queue leaves ordinary memory context and tools available without overwriting the queue', t => {
  const f = fixture(t);
  f.maintenance.dispose();
  const previous = process.env.DSH_HOME;
  process.env.DSH_HOME = f.home;
  t.after(() => {
    if (previous === undefined) delete process.env.DSH_HOME;
    else process.env.DSH_HOME = previous;
  });
  f.store.writeFile('maintenance.json', 'broken queue', 1048576);
  const tools = new Map(), contexts = [], warnings = [];
  apply({
    llm: {}, logger: {warn: text => warnings.push(text)}, on: () => {}, effect: factory => factory(),
    tools: {register: tool => tools.set(tool.name, tool)},
    systemPrompt: {section: () => {}, context: value => contexts.push(value)},
  }, {enabled: true, ...timing});
  assert.equal(tools.size, 3);
  assert.ok(contexts[0].text().includes('No saved memory'));
  assert.equal(warnings.length, 1);
  assert.equal(f.store.readFile('maintenance.json', 1048576), 'broken queue');
});

const event = (seq, type, data) => ({seq, type, data});
const userEvent = (seq, text, kind = 'user') => event(seq, 'user/message', message('u' + seq, 'user', text, kind));
const assistantEvent = (seq, text) => event(seq, 'assistant/message', {message: message('a' + seq, 'assistant', text)});
const callEvent = (seq, name, args = '{}') => event(seq, 'tool/call', {callId: 'call-' + seq, name, arguments: args});
const resultEvent = (seq, callSeq, text, isError = false) => event(seq, 'tool/result', {message: {content: [{type: 'tool-result', toolCallId: 'call-' + callSeq, isError, content: [{type: 'text', text}]}]}});

test('structured DSH snapshots preserve scoped user instructions, tool verification, failures and source metadata', async t => {
  const f = fixture(t);
  f.session.header.cwd = '/work/csgclaw';
  f.session.snapshotEvents = () => [
    event(0, 'turn/start', {turn: 0}),
    userEvent(1, 'This time do not create a PR. Fix the build.'),
    userEvent(2, 'Injected memory must not be evidence.', 'plugin'),
    event(3, 'system/message', {message: {content: [{type: 'text', text: 'System instructions must not leak.'}]}}),
    callEvent(4, 'exec_command', '{"cmd":"pnpm test"}'), resultEvent(5, 4, 'ERROR Missing dependency', true),
    callEvent(6, 'exec_command', '{"cmd":"pnpm install && pnpm test"}'), resultEvent(7, 6, '62 tests passed'),
    callEvent(8, 'memory_read'), resultEvent(9, 8, 'Historical memory must not be evidence.'),
    assistantEvent(10, 'I think the deployment is also fixed (not tested).'), event(11, 'turn/end', completed.data),
  ];
  f.maintenance.capture(f.session, completed);
  await f.advance(11100);
  const input = JSON.parse(f.calls[0].messages[0].content[0].text);
  assert.equal(input.cwd, '/work/csgclaw');
  assert.equal(input.session_id, f.session.id);
  assert.ok(input.conversation.includes('[event 1] user: This time'));
  assert.ok(input.conversation.includes('pnpm test'));
  assert.ok(input.conversation.includes('tool result exec_command (error): ERROR Missing dependency'));
  assert.ok(input.conversation.includes('62 tests passed'));
  assert.ok(input.conversation.includes('not tested'));
  assert.ok(!input.conversation.includes('must not'));
  const merge = JSON.parse(f.calls[1].messages[0].content[0].text);
  assert.ok(f.store.readFile(merge.source_file, 65536).includes('Workspace: /work/csgclaw'));
  assert.ok(f.store.readHandbook().content.includes('Reply preferences'));
});

test('unsuccessful direct-user turns retain reusable failure evidence rather than claiming success', async t => {
  const f = fixture(t);
  const failed = {type: 'turn/end', data: {reason: {kind: 'error', error: {message: 'Build failed: missing compiler', code: 'BUILD'}}}};
  f.session.snapshotEvents = () => [event(0, 'turn/start', {turn: 0}), userEvent(1, 'Fix the compiler issue.'), assistantEvent(2, 'The proposed fix is unverified.'), event(3, 'turn/end', failed.data)];
  f.maintenance.capture(f.session, failed);
  await f.advance(11100);
  assert.ok(f.state().checkpoints[f.session.id].cursor === 'event:3');
  const input = JSON.parse(f.calls[0].messages[0].content[0].text);
  assert.ok(input.conversation.includes('turn ended: error'));
  assert.ok(input.conversation.includes('missing compiler'));
});

test('event cursors isolate new turns and migrate already-saved message cursors', async t => {
  const f = fixture(t);
  // The first snapshot uses the earlier plugin's message-id cursor.
  f.maintenance.capture(f.session, completed);
  let events = [userEvent(0, 'Old preference.'), event(1, 'assistant/message', {message: message('a1', 'assistant', 'Old reply.')}), event(2, 'turn/start', {turn: 1}), userEvent(3, 'New project convention.'), callEvent(4, 'verify'), resultEvent(5, 4, 'Verified'), event(6, 'turn/end', completed.data)];
  f.session.snapshotEvents = () => events;
  f.setNow(11000);
  f.maintenance.capture(f.session, completed);
  assert.ok(!f.state().pending[1].transcript.includes('Old preference'));
  assert.ok(f.state().pending[1].transcript.includes('New project convention'));
  events = [...events, event(7, 'turn/start', {turn: 2}), userEvent(8, 'Latest correction.'), event(9, 'turn/end', completed.data)];
  f.setNow(12000);
  f.maintenance.capture(f.session, completed);
  assert.ok(f.state().pending[2].transcript.includes('Latest correction'));
  assert.ok(!f.state().pending[2].transcript.includes('New project convention'));
});

test('large tool transcripts preserve the latest user scope and never persist raw secrets or reasoning', t => {
  const f = fixture(t);
  const events = [event(0, 'turn/start', {turn: 0}), userEvent(1, 'Only this project uses npm.')];
  for (let index = 0; index < 16; index++) {
    const seq = 2 + index * 2;
    events.push(callEvent(seq, 'verify'), resultEvent(seq + 1, seq, 'x'.repeat(20000) + '\npassword=do-not-save-me'));
  }
  events.push(event(34, 'assistant/message', {message: {...message('a', 'assistant', 'Proposed fix.'), content: [{type: 'reasoning', text: 'Hidden reasoning.'}, {type: 'text', text: 'Proposed fix.'}]}}));
  events.push(event(35, 'turn/end', completed.data));
  f.session.snapshotEvents = () => events;
  f.maintenance.capture(f.session, completed);
  const transcript = f.state().pending[0].transcript;
  assert.ok(Buffer.byteLength(transcript) <= 32768);
  assert.ok(transcript.includes('Only this project uses npm.'));
  assert.ok(!transcript.includes('do-not-save-me'));
  assert.ok(!transcript.includes('Hidden reasoning'));
});

test('retrieval uses the detailed handbook while explicit replacement forgets facts in both current layers', async t => {
  const f = fixture(t);
  f.maintenance.capture(f.session, completed);
  await f.advance(11100);
  assert.ok(f.store.search('work documents').some(match => match.document === 'MEMORY.md'));
  f.store.writeFile('rollout_summaries/old.md', 'Forgotten old fact.', 65536);
  f.store.update('Only the new approved fact.', f.store.read().revision);
  f.maintenance.manualUpdated();
  assert.equal(f.store.readHandbook().content, 'Only the new approved fact.');
  assert.deepEqual(f.store.search('Chinese'), []);
  assert.deepEqual(f.store.search('Forgotten'), []);
  assert.throws(() => f.store.search('fact', 11), /Invalid/);
});

test('consolidation may forget all content and never saves incomplete or oversized document pairs', async t => {
  let response = '{"summary":"invalid: missing handbook"}';
  const f = fixture(t, options => options.system.includes('extraction')
    ? {memory: 'The user asks to forget all preferences.', rollout_summary: 'Explicit forgetting request.'}
    : response);
  f.store.update('Old preference.', f.store.read().revision);
  f.maintenance.capture(f.session, completed);
  await f.advance(11100);
  assert.equal(f.store.read().content, 'Old preference.');
  assert.equal(f.store.readHandbook().content, 'Old preference.');
  response = {summary: 'New valid summary.', handbook: 'x'.repeat(65537)};
  await f.advance(11150);
  assert.equal(f.store.read().content, 'Old preference.');
  assert.equal(f.store.readHandbook().content, 'Old preference.');
  response = {summary: '', handbook: ''};
  await f.advance(11200);
  assert.equal(f.store.read().content, '');
  assert.equal(f.store.readHandbook().content, '');
  assert.equal(f.state().pending.length, 0);
});

test('a concurrent handbook change retries consolidation without overwriting the newer evidence', async t => {
  let f, merges = 0;
  f = fixture(t, options => {
    if (options.system.includes('extraction')) return {memory: 'New verified fact.', rollout_summary: 'Verification.'};
    const input = JSON.parse(options.messages[0].content[0].text);
    if (++merges === 1) f.store.writeFile('MEMORY.md', 'Concurrent evidence.', 65536);
    return {summary: 'New verified fact.', handbook: input.current_handbook + '\nNew verified fact.'};
  });
  f.maintenance.capture(f.session, completed);
  await f.advance(11100);
  assert.equal(f.store.read().content, '');
  assert.equal(f.store.readHandbook().content, 'Concurrent evidence.');
  await f.advance(11150);
  assert.ok(f.store.readHandbook().content.includes('Concurrent evidence.'));
  assert.ok(f.store.readHandbook().content.includes('New verified fact.'));
});

test('saved extraction from the earlier policy is re-extracted before entering the new handbook', async t => {
  const f = fixture(t);
  f.maintenance.capture(f.session, completed);
  f.maintenance.dispose();
  const state = f.state();
  state.pending[0].extraction = {memory: 'Old unsupported universal preference.', rollout_summary: 'Earlier simple policy.'};
  f.store.writeFile('maintenance.json', JSON.stringify(state), 1048576);
  f.restart();
  await f.advance(11100);
  assert.equal(f.calls.length, 2);
  assert.ok(!JSON.stringify(f.calls).includes('Old unsupported universal preference'));
  assert.ok(f.store.readHandbook().content.includes('Reply preferences'));
});

test('an injected-only root turn cannot reuse a prior direct turn as new evidence', t => {
  const f = fixture(t);
  f.session.snapshotEvents = () => [event(0, 'turn/start', {turn: 0}), userEvent(1, 'Earlier direct request.'), event(2, 'turn/end', completed.data)];
  f.maintenance.capture(f.session, completed);
  f.session.snapshotEvents = () => [event(0, 'turn/start', {turn: 0}), userEvent(1, 'Earlier direct request.'), event(2, 'turn/end', completed.data), event(3, 'turn/start', {turn: 1}), userEvent(4, 'Synthetic maintenance.', 'plugin'), callEvent(5, 'verify'), resultEvent(6, 5, 'Synthetic proof'), event(7, 'turn/end', completed.data)];
  f.setNow(11000);
  f.maintenance.capture(f.session, completed);
  assert.equal(f.state().pending.length, 1);
  assert.ok(!JSON.stringify(f.state()).includes('Synthetic'));
});

test('quoted and escaped JSON credentials are redacted while non-secret evidence stays valid', () => {
  const secrets = ['fixture-password-680', 'fixture-token-680', 'fixture-client-680', 'fixture-quoted-680"\\line\nend'];
  const input = {
    password: secrets[0], access_token: secrets[1], nested: {clientSecret: secrets[2]},
    payload: JSON.stringify({password: secrets[3], result: '62 tests passed'}), count: 62,
  };
  const redacted = redactMemory(JSON.stringify(input));
  for (const secret of secrets) assert.ok(!redacted.includes(secret) && !redacted.includes(JSON.stringify(secret).slice(1, -1)));
  const parsed = JSON.parse(redacted);
  assert.equal(parsed.password, '[REDACTED]');
  assert.equal(parsed.access_token, '[REDACTED]');
  assert.equal(parsed.nested.clientSecret, '[REDACTED]');
  assert.equal(JSON.parse(parsed.payload).password, '[REDACTED]');
  assert.equal(JSON.parse(parsed.payload).result, '62 tests passed');
  assert.equal(parsed.count, 62);
  assert.ok(!redactMemory('tool result: {"password":"fixture-password-680","ok":true}').includes(secrets[0]));
  const quoted = 'tool result: ' + JSON.stringify(JSON.stringify(input));
  const safeQuoted = redactMemory(quoted);
  assert.ok(!safeQuoted.includes('fixture-password-680'));
  assert.ok(safeQuoted.includes('62 tests passed'));
  const unwrapped = 'tool result: ' + JSON.stringify(JSON.stringify({password: secrets[3]})).slice(1, -1);
  assert.ok(!redactMemory(unwrapped).includes('fixture-quoted-680'));
  const escapedKey = '{"pass\\u0077ord":"fixture-password-680","ok":true}';
  assert.equal(JSON.parse(redactMemory(escapedKey)).password, '[REDACTED]');
});

test('JSON tool arguments and results never reach persisted snapshots or maintenance model requests with credentials', async t => {
  const password = 'fixture-password-evidence-680', token = 'fixture-token-evidence-680';
  const f = fixture(t);
  f.session.snapshotEvents = () => [event(0, 'turn/start', {turn: 0}), userEvent(1, 'Check the build.'),
    callEvent(2, 'verify', JSON.stringify({password, command: 'pnpm test'})),
    resultEvent(3, 2, 'Verification result: ' + JSON.stringify({access_token: token, result: '62 tests passed'})),
    event(4, 'turn/end', completed.data)];
  f.maintenance.capture(f.session, completed);
  const persisted = f.store.readFile('maintenance.json', 1048576);
  assert.ok(!persisted.includes(password));
  assert.ok(!persisted.includes(token));
  assert.ok(persisted.includes('62 tests passed'));
  f.restart();
  await f.advance(11100);
  assert.ok(!JSON.stringify(f.calls).includes(password));
  assert.ok(!JSON.stringify(f.calls).includes(token));
});

test('forgetting in another conversation advances discarded buffered cursors across restart', async t => {
  for (const mode of ['messages', 'events']) {
    await t.test(mode, async t => {
      const forgotten = 'Blue dashboards are my lasting preference.';
      const f = fixture(t, options => {
        const input = JSON.parse(options.messages[0].content[0].text);
        if (options.system.includes('extraction')) return input.conversation.includes(forgotten)
          ? {memory: forgotten, rollout_summary: 'Old preference.'} : {memory: '', rollout_summary: ''};
        return {summary: forgotten, handbook: forgotten};
      });
      let messages = [message('u1', 'user', 'Initial context.'), message('a1', 'assistant', 'Acknowledged.')];
      let events = [event(0, 'turn/start', {turn: 0}), userEvent(1, 'Initial context.'), assistantEvent(2, 'Acknowledged.'), event(3, 'turn/end', completed.data)];
      f.session.deriveMessages = () => messages;
      if (mode === 'events') f.session.snapshotEvents = () => events;
      f.maintenance.capture(f.session, completed);
      f.setNow(10500);
      messages = [...messages, message('u2', 'user', forgotten), message('a2', 'assistant', 'Acknowledged.')];
      events = [...events, event(4, 'turn/start', {turn: 1}), userEvent(5, forgotten), assistantEvent(6, 'Acknowledged.'), event(7, 'turn/end', completed.data)];
      f.maintenance.capture(f.session, completed);
      assert.ok(f.state().buffered[f.session.id].transcript.includes(forgotten));
      // memory_update from conversation B uses the same Agent-wide controller.
      f.store.update('Current approved memory.', f.store.read().revision);
      f.maintenance.manualUpdated();
      assert.equal(f.state().checkpoints[f.session.id].at, 10000);
      f.restart();
      messages = [...messages, message('u3', 'user', 'Hello again.'), message('a3', 'assistant', 'Hello.')];
      events = [...events, event(8, 'turn/start', {turn: 2}), userEvent(9, 'Hello again.'), assistantEvent(10, 'Hello.'), event(11, 'turn/end', completed.data)];
      f.setNow(11000);
      f.maintenance.capture(f.session, completed);
      assert.ok(!f.state().pending[0].transcript.includes(forgotten));
      await f.advance(12100);
      assert.equal(f.store.read().content, 'Current approved memory.');
      assert.equal(f.store.readHandbook().content, 'Current approved memory.');
    });
  }
});

test('saved pending and buffered evidence from the earlier redactor is sanitized before reuse', async t => {
  const secret = 'fixture-password-legacy-680';
  const f = fixture(t);
  f.maintenance.capture(f.session, completed);
  f.maintenance.dispose();
  const state = f.state();
  state.pending[0].transcript += '\nLegacy result: ' + JSON.stringify({password: secret});
  state.buffered[f.session.id] = {sessionId: f.session.id, cursor: 'a2', route: {provider: 'p', model: 'm'}, transcript: 'user: '+JSON.stringify({access_token: secret})};
  f.store.writeFile('maintenance.json', JSON.stringify(state), 1048576);
  f.restart();
  assert.ok(!f.store.readFile('maintenance.json', 1048576).includes(secret));
  await f.advance(11100);
  assert.ok(!JSON.stringify(f.calls).includes(secret));
});

test('JSON credentials in generated fields are sanitized without breaking the model response contract', async t => {
  const secret = 'fixture-generated-680"\\tail\nend';
  const f = fixture(t, options => options.system.includes('extraction')
    ? {memory: 'Verified fact. ' + JSON.stringify({password: secret}), rollout_summary: 'Verified result.'}
    : {summary: 'Verified fact. ' + JSON.stringify({access_token: secret}), handbook: 'Verified procedure. ' + JSON.stringify({client_secret: secret})});
  f.maintenance.capture(f.session, completed);
  const id = f.state().pending[0].id;
  await f.advance(11100);
  assert.equal(f.state().pending.length, 0);
  for (const text of [f.store.read().content, f.store.readHandbook().content, f.store.readFile('rollout_summaries/' + id + '.md', 65536)]) {
    assert.ok(!text.includes('fixture-generated-680'));
    assert.ok(text.includes('Verified'));
    assert.ok(text.includes('[REDACTED]'));
  }
});
