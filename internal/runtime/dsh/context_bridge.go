package dsh

// Forward native DSH events that ACP does not expose while a turn is running.
const contextBridgeModule = `import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
import { randomBytes } from 'node:crypto';
import { writeFile, unlink } from 'node:fs/promises';
export const inject = ['agents', 'compaction', 'systemPrompt'];
export const name = 'csgclaw-context';
function notify(sessionId, update) {
  process.stdout.write(JSON.stringify({jsonrpc:'2.0',method:'csgclaw/context',params:{sessionId,update}}) + '\n');
}

function forwardReasoning(ctx) {
  const attempts = new WeakMap();
  function flush(session, attempt) {
    clearTimeout(attempt.timer);
    attempt.timer = undefined;
    if (attempt.pending === '') return;
    notify(session.header.id, {sessionUpdate:'reasoning_delta', text:attempt.pending});
    attempt.pending = '';
  }
  ctx.on('agent/assistant-stream', ({agent, frame}) => {
    const session = agent.session;
    if (frame.type === 'start') {
      attempts.set(session, {id:frame.attemptId, turn:frame.turn, step:frame.step,
        blocks:new Map(), pending:''});
      return;
    }
    const attempt = attempts.get(session);
    if (attempt?.id !== frame.attemptId) return;
    if (frame.type === 'end') {
      flush(session, attempt);
      attempts.delete(session);
      return;
    }
    if (frame.type !== 'chunk') return;
    const chunk = frame.chunk;
    const isReasoning = chunk.type === 'reasoning-delta' ||
      (chunk.type === 'block-start' && chunk.blockType === 'reasoning') ||
      (chunk.type === 'block-end' && chunk.block.type === 'reasoning');
    if (!isReasoning) return;
    let block = attempt.blocks.get(chunk.index);
    if (!block) {
      block = {sent:''};
      attempt.blocks.set(chunk.index, block);
    }
    if (block.final !== undefined) return;
    if (chunk.type === 'block-end') block.final = chunk.block.text;
    if (chunk.type !== 'reasoning-delta' || chunk.text === '') return;
    block.sent += chunk.text;
    attempt.pending += chunk.text;
    attempt.timer ??= setTimeout(() => flush(session, attempt), 40);
  });
  ctx.on('session/event', (session, event) => {
    if (event.type !== 'assistant/message') return;
    const attempt = attempts.get(session);
    if (!attempt || attempt.turn !== event.data.turn || attempt.step !== event.data.step) return;
    flush(session, attempt);
    // Use the assembler's first-seen block order. DSH omits whitespace-only
    // blocks on interruption, and ACP skips empty reasoning blocks.
    const visibleBlocks = [...attempt.blocks.values()].filter(block => {
      const text = block.final ?? block.sent;
      return (event.data.interrupted ? text.trim() : text) !== '';
    });
    const prefixBytes = event.data.message.content
      .filter(block => block.type === 'reasoning' && block.text !== '')
      .map((block, index) => {
        const sent = visibleBlocks[index]?.sent ?? '';
        return block.text.startsWith(sent) ? Buffer.byteLength(sent) : 0;
      });
    // ACP projects the committed message onto its ordered update queue. Mark
    // the exact message before its native thought chunks replay the same text.
    if (prefixBytes.some(size => size > 0)) {
      notify(session.header.id, {sessionUpdate:'reasoning_commit', messageId:event.data.message.id, prefixBytes});
    }
  });
}

export function apply(ctx) {
  ctx.systemPrompt.section({name: 'csgclaw:standing-instructions', order: 9400, interpolate: false,
    text: () => readFileSync(new URL('./standing-instructions.txt', import.meta.url), 'utf8')});
  const token = randomBytes(32).toString('hex');
  const descriptor = new URL('./context-control.json', import.meta.url);
  const jobs = new Set();
  const server = createServer(async (req, res) => {
    res.setHeader('Content-Type','application/x-ndjson');
    const reply = (status, usage, reason) => { res.end(JSON.stringify({status, usage, reason}) + '\n'); };
    if (req.method !== 'POST' || req.url !== '/compact' || req.headers.authorization !== 'Bearer ' + token) { res.statusCode=403; return res.end(); }
    let body='';
    for await (const part of req) { body+=part; if(body.length>4096){res.statusCode=413;return res.end();} }
    let session;
    try { session=JSON.parse(body).session_id; } catch { return reply('unavailable'); }
    const agent=ctx.agents.get(session);
    if (!agent) return reply('unavailable', undefined, 'session_restore_failed');
    const controller=new AbortController();
    const dispose=ctx.on('session/event', (activeSession,event) => {
      if (activeSession.header.id === session && event.type === 'compaction/start') {
        res.write(JSON.stringify({status:'running'}) + '\n');
      }
    });
    const job=ctx.compaction.compactNow(agent,controller.signal);
    jobs.add(job);
    try {
      const result=await job;
      if (!result) return reply('no_history');
      let used=null;
      try { used=ctx.get('tokenMeter')?.measure(agent.session).totalTokens ?? null; } catch {}
      reply('completed', {session_id:session, used_tokens:used, estimated:true});
    } catch(error) {
      const status={busy:'busy',cancelled:'interrupted',changed:'interrupted',summary:'failed',commit:'uncertain',persistence:'uncertain'}[error?.code] ?? 'failed';
      reply(status);
    } finally {dispose();jobs.delete(job);}
  });
  ctx.effect(async () => {
    await new Promise((resolve,reject) => {server.once('error',reject);server.listen(0,'127.0.0.1',resolve);});
    await writeFile(descriptor,JSON.stringify({port:server.address().port,token}),{mode:0o600});
    return async () => {server.close();await Promise.allSettled([...jobs]);await unlink(descriptor).catch(()=>{});};
  });
  ctx.on('agent/error', ({agent, error}) => {
    for (let e = error, depth = 0; e && depth < 8; e = e.cause, depth++) {
      if (e.code === 'CONTEXT_WINDOW_EXCEEDED') {
        process.stdout.write(JSON.stringify({jsonrpc:'2.0',method:'csgclaw/context',params:{sessionId:agent.session.id,update:{sessionUpdate:'context_error'}}}) + '\n');
        break;
      }
    }
  });
  forwardReasoning(ctx);
  ctx.on('session/event', (session, event) => {
    if (event.type === 'turn/end') {
      notify(session.header.id, {sessionUpdate:'turn_end', reason:event.data.reason});
      return;
    }
    if (event.type === 'assistant/message' || event.type === 'compaction/summary') {
      const params = { sessionId: session.header.id, update: { sessionUpdate: 'token_usage', requestId: String(event.seq), usage: event.data.usage ?? null, interrupted: Boolean(event.data.interrupted) } };
      process.stdout.write(JSON.stringify({ jsonrpc: '2.0', method: 'csgclaw/context', params }) + '\n');
      return;
    }
    if (event.type !== 'compaction/start' && event.type !== 'compaction/end') return;
    const compacting = event.type === 'compaction/start';
    const params = { sessionId: session.header.id, update: { sessionUpdate: 'context_compaction', compacting, failed: Boolean(event.data.error) } };
    if (!compacting) {
      try {
        const size = session.requestContext()?.contextWindow;
        const meter = ctx.get('tokenMeter');
        if (size > 0 && meter) { params.update.size = size; params.update.used = meter.measure(session).totalTokens; }
      } catch { /* Keep usage unknown until the next native usage update. */ }
    }
    process.stdout.write(JSON.stringify({ jsonrpc: '2.0', method: 'csgclaw/context', params }) + '\n');
  });
}
`
