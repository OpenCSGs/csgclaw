package dsh

// The native ACP adapter does not project compaction lifecycle events. This
// small managed plugin forwards those existing events without owning compaction.
const contextBridgeModule = `import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
import { randomBytes } from 'node:crypto';
import { writeFile, unlink } from 'node:fs/promises';
export const inject = ['agents', 'compaction', 'systemPrompt'];
export const name = 'csgclaw-context';
export function apply(ctx) {
  ctx.systemPrompt.section({name: 'csgclaw:standing-instructions', order: 9400, interpolate: false,
    text: () => readFileSync(new URL('./standing-instructions.txt', import.meta.url), 'utf8')});
  const token = randomBytes(32).toString('hex');
  const descriptor = new URL('./context-control.json', import.meta.url);
  const jobs = new Set();
  const server = createServer(async (req, res) => {
    const reply = (status, usage) => { res.setHeader('Content-Type','application/json'); res.end(JSON.stringify({status, usage})); };
    if (req.method !== 'POST' || req.url !== '/compact' || req.headers.authorization !== 'Bearer ' + token) { res.statusCode=403; return res.end(); }
    let body='';
    for await (const part of req) { body+=part; if(body.length>4096){res.statusCode=413;return res.end();} }
    let session;
    try { session=JSON.parse(body).session_id; } catch { return reply('unavailable'); }
    const agent=ctx.agents.get(session);
    if (!agent) return reply('unavailable');
    const controller=new AbortController();
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
    } finally {jobs.delete(job);}
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
  ctx.on('session/event', (session, event) => {
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
