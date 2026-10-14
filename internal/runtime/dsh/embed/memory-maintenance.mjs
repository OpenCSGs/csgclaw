import crypto from 'node:crypto';
import {maxMemoryBytes, maxHandbookBytes, revision} from './memory-store.mjs';
import {extractionPolicy, consolidationPolicy} from './memory-policy.mjs';
import {captureConversation, redactMemory} from './memory-evidence.mjs';
export {redactMemory} from './memory-evidence.mjs';

const maxStateBytes = 4 * 1024 * 1024;
const maxTranscriptBytes = 32 * 1024;
const maxPending = 16;
const stateFile = 'maintenance.json';
function decodeState(raw) {
  if (!raw) return {version: 1, checkpoints: {}, pending: [], buffered: {}, manualGeneration: 0};
  const state = JSON.parse(raw);
  if (state.version !== 1 || !state.checkpoints || typeof state.checkpoints !== 'object' || Array.isArray(state.checkpoints) || Object.keys(state.checkpoints).length > 128 || !Array.isArray(state.pending) || state.pending.length > maxPending || !Number.isSafeInteger(state.manualGeneration) || state.manualGeneration < 0) throw new Error('Invalid memory maintenance state');
  for (const checkpoint of Object.values(state.checkpoints)) {
    if (!checkpoint || !Number.isFinite(checkpoint.at) || typeof checkpoint.cursor !== 'string') throw new Error('Invalid memory checkpoint cursor');
  }
  state.buffered ??= {};
  if (!state.buffered || typeof state.buffered !== 'object' || Array.isArray(state.buffered) || Object.keys(state.buffered).length > maxPending) throw new Error('Invalid buffered memory state');
  for (const source of Object.values(state.buffered)) {
    if (!source || typeof source.sessionId !== 'string' || typeof source.cursor !== 'string' || typeof source.transcript !== 'string' || Buffer.byteLength(source.transcript) > maxTranscriptBytes || typeof source.route?.provider !== 'string' || typeof source.route?.model !== 'string') throw new Error('Invalid buffered memory conversation');
    if (source.cwd !== undefined && (typeof source.cwd !== 'string' || source.cwd.length > 4096)) throw new Error('Invalid memory workspace');
  }
  for (const job of state.pending) {
    if (!/^[a-f0-9]{24}$/.test(job.id) || typeof job.sessionId !== 'string' || typeof job.transcript !== 'string' || Buffer.byteLength(job.transcript) > maxTranscriptBytes || !Number.isFinite(job.dueAt) || !Number.isFinite(job.capturedAt) || !Number.isSafeInteger(job.manualGeneration) || typeof job.route?.provider !== 'string' || typeof job.route?.model !== 'string') throw new Error('Invalid memory checkpoint');
    if (job.cwd !== undefined && (typeof job.cwd !== 'string' || job.cwd.length > 4096)) throw new Error('Invalid memory workspace');
    if (job.extraction && (typeof job.extraction.memory !== 'string' || typeof job.extraction.rollout_summary !== 'string' || Buffer.byteLength(job.extraction.memory) > maxMemoryBytes || Buffer.byteLength(job.extraction.rollout_summary) > 8192)) throw new Error('Invalid saved memory extraction');
  }
  return state;
}

// Runs auxiliary LLM requests through DSH's configured provider, never through an Agent/tool loop.
export function startMemoryMaintenance(ctx, store, config, clock = {}) {
  const interval = config.checkpointIntervalMs;
  const idle = config.checkpointIdleMs;
  const retry = config.retryDelayMs;
  const timeout = config.requestTimeoutMs;
  for (const value of [interval, idle, retry, timeout]) {
    if (!Number.isSafeInteger(value) || value <= 0) throw new Error('Invalid memory maintenance timing');
  }
  const now = clock.now ?? Date.now;
  const schedule = clock.setTimeout ?? setTimeout;
  const unschedule = clock.clearTimeout ?? clearTimeout;
  const storedState = store.readFile(stateFile, maxStateBytes);
  const safeState = redactMemory(storedState);
  const state = decodeState(safeState);
  let disposed = false, busy = false, timer, activeAbort;
  const save = () => store.writeFile(stateFile, JSON.stringify(state), maxStateBytes);
  // Upgrade pending evidence written by the previous redactor before it can be
  // reused or sent to the provider. Invalid queues remain untouched for recovery.
  if (storedState && safeState !== storedState) save();
  const warn = () => ctx.logger.warn('DSH background memory maintenance failed; its checkpoint remains queued for retry.');
  const queue = source => {
    const capturedAt = now();
    const id = revision(source.sessionId + '\0' + source.cursor + '\0' + capturedAt).slice(0, 24);
    state.pending.push({id, sessionId: source.sessionId, route: source.route, transcript: source.transcript, cwd: source.cwd, capturedAt, dueAt: capturedAt + idle, manualGeneration: state.manualGeneration});
    state.checkpoints[source.sessionId] = {at: capturedAt, cursor: source.cursor};
    delete state.buffered[source.sessionId];
    state.checkpoints = Object.fromEntries(Object.entries(state.checkpoints).sort((a, b) => b[1].at - a[1].at).slice(0, 128));
  };
  const flushBuffered = () => {
    if (disposed) return;
    let changed = false;
    for (const source of Object.values(state.buffered)) {
      if (state.pending.length >= maxPending) break;
      if (now() < (state.checkpoints[source.sessionId]?.at ?? 0) + interval) continue;
      queue(source);
      changed = true;
    }
    if (changed) save();
  };
  const arm = () => {
    if (timer !== undefined) unschedule(timer);
    timer = undefined;
    if (disposed || busy) return;
    const dueTimes = state.pending.map(job => job.dueAt);
    if (state.pending.length < maxPending) {
      for (const source of Object.values(state.buffered)) dueTimes.push((state.checkpoints[source.sessionId]?.at ?? 0) + interval);
    }
    if (dueTimes.length === 0) return;
    const due = Math.min(...dueTimes);
    timer = schedule(() => { timer = undefined; void runDue().catch(warn); }, Math.max(0, Math.min(due - now(), 2147483647)));
    timer?.unref?.();
  };
  const call = async (job, system, input) => {
    const abort = new AbortController();
    activeAbort = abort;
    let deadline;
    const expired = new Promise((_, reject) => {
      deadline = schedule(() => { abort.abort(); reject(new Error('Memory request timed out')); }, timeout);
    });
    const generate = async () => {
      const blocks = new Map();
      let finish;
      for await (const chunk of ctx.llm.stream({
        provider: config.model ? 'deepseek-official' : job.route.provider,
        model: config.model || job.route.model, signal: abort.signal,
        maxTokens: system === extractionPolicy ? 8192 : 32768, tools: [], sessionId: job.sessionId, system,
        messages: [{id: crypto.randomUUID(), role: 'user', source: {kind: 'plugin', plugin: 'csgclaw-memory'}, content: [{type: 'text', text: redactMemory(JSON.stringify(input))}]}],
      })) {
        abort.signal.throwIfAborted();
        if (chunk.type === 'tool-call-delta' || (chunk.type === 'block-end' && chunk.block.type === 'tool-call')) throw new Error('Memory requests cannot call tools');
        if (chunk.type === 'text-delta') blocks.set(chunk.index, (blocks.get(chunk.index) ?? '') + chunk.text);
        if (chunk.type === 'block-end' && chunk.block.type === 'text') blocks.set(chunk.index, chunk.block.text);
        if (chunk.type === 'finish') finish = chunk.reason.kind;
        if ([...blocks.values()].reduce((size, text) => size + Buffer.byteLength(text), 0) > maxMemoryBytes + maxHandbookBytes + 16384) throw new Error('Memory response is too large');
      }
      if (finish !== 'stop') throw new Error('Memory response did not complete');
      return JSON.parse(redactMemory([...blocks.values()].join('')).trim());
    };
    try { return await Promise.race([generate(), expired]); }
    finally { unschedule(deadline); activeAbort = undefined; }
  };
  const stillCurrent = job => !disposed && job.manualGeneration === state.manualGeneration && state.pending.includes(job);
  const processJob = async job => {
    if (!job.extraction || job.extraction.policyVersion !== 2) {
      const value = await call(job, extractionPolicy, {session_id: job.sessionId, cwd: job.cwd ?? '(unknown)', captured_at: new Date(job.capturedAt).toISOString(), conversation: job.transcript, limits: {memory_bytes: maxMemoryBytes, rollout_summary_bytes: 8192}});
      if (typeof value.memory !== 'string' || typeof value.rollout_summary !== 'string' || Buffer.byteLength(value.memory) > maxMemoryBytes || Buffer.byteLength(value.rollout_summary) > 8192) throw new Error('Invalid memory extraction');
      if (!stillCurrent(job)) return;
      job.extraction = {memory: value.memory, rollout_summary: value.rollout_summary, policyVersion: 2};
      save();
    }
    // Codex's minimum-signal gate: empty facts cause no evidence or document writes.
    if (!job.extraction.memory.trim()) return;
    const sourceFile = 'rollout_summaries/' + job.id + '.md';
    const report = '# DSH memory checkpoint\n\nSession: ' + job.sessionId + '\nWorkspace: ' + (job.cwd ?? '(unknown)') + '\nCaptured: ' + new Date(job.capturedAt).toISOString() + '\n\n' + job.extraction.rollout_summary + '\n\n## Durable facts\n\n' + job.extraction.memory + '\n';
    store.writeFile(sourceFile, report, maxMemoryBytes + 16384);
    const current = store.read();
    const handbook = store.readHandbook();
    const merged = await call(job, consolidationPolicy, {current_summary: current.content, current_handbook: handbook.content, new_facts: job.extraction.memory, source_file: sourceFile, source_summary: job.extraction.rollout_summary, limits: {summary_bytes: maxMemoryBytes, handbook_bytes: maxHandbookBytes}});
    if (!stillCurrent(job)) return;
    if (typeof merged.summary !== 'string' || typeof merged.handbook !== 'string') throw new Error('Invalid consolidated memory');
    store.updateDocuments(merged, {summary: current.revision, handbook: handbook.revision});
  };
  async function runDue() {
    if (disposed || busy) return;
    busy = true;
    try {
      flushBuffered();
      for (const job of [...state.pending]) {
        if (!stillCurrent(job) || job.dueAt > now()) continue;
        try {
          await processJob(job);
          if (stillCurrent(job)) state.pending.splice(state.pending.indexOf(job), 1);
        } catch (error) {
          // Provider text may contain credentials; retain only a generic failure and never log the raw exception.
          if (stillCurrent(job)) {
            job.dueAt = now() + retry;
            job.lastError = 'memory_generation_failed';
            warn();
          }
        }
        if (!disposed) save();
      }
      flushBuffered();
    } finally { busy = false; arm(); }
  }
  const capture = (session, event) => {
    if (disposed || event.type !== 'turn/end' || session.header.origin === 'subagent' || (session.header.delegationDepth ?? 0) > 0) return;
    const route = session.requestContext();
    if (!route?.provider || !route.model) return;
    const checkpoint = state.checkpoints[session.id];
    const captured = captureConversation(session, checkpoint?.cursor, maxTranscriptBytes);
    if (!captured?.transcript.trim()) return;
    const source = {sessionId: session.id, ...captured, cwd: redactMemory(String(session.header.cwd ?? '(unknown)')).slice(0, 4096), route: {provider: route.provider, model: route.model}};
    if ((!checkpoint || now() - checkpoint.at >= interval) && state.pending.length < maxPending) queue(source);
    else {
      if (!state.buffered[session.id] && Object.keys(state.buffered).length >= maxPending) return;
      state.buffered[session.id] = source;
    }
    save();
    arm();
  };
  const manualUpdated = () => {
    // Explicit in-turn changes take priority over facts captured before that change, including forgetting requests.
    // Discarding buffered text must also consume its cursor; otherwise another
    // conversation can reread that text after an Agent-wide forgetting request.
    const discarded = new Set(Object.keys(state.buffered));
    for (const [sessionId, source] of Object.entries(state.buffered)) {
      state.checkpoints[sessionId] = {at: state.checkpoints[sessionId]?.at ?? now(), cursor: source.cursor};
    }
    state.checkpoints = Object.fromEntries(Object.entries(state.checkpoints).sort((a, b) => Number(discarded.has(b[0])) - Number(discarded.has(a[0])) || b[1].at - a[1].at).slice(0, 128));
    state.manualGeneration++;
    state.pending = [];
    state.buffered = {};
    activeAbort?.abort();
    save();
    arm();
  };
  const dispose = () => {
    disposed = true;
    if (timer !== undefined) unschedule(timer);
    activeAbort?.abort();
  };
  arm();
  return {capture, runDue, manualUpdated, dispose};
}
