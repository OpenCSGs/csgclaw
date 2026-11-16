package dsh

// The native ACP adapter does not project compaction lifecycle events. This
// managed plugin adds CSGClaw's standing instructions and forwards those events.
const contextBridgeModule = `import {readFileSync} from 'node:fs';
export const inject = ['systemPrompt'];
export const name = 'csgclaw-context';
export function apply(ctx) {
  ctx.systemPrompt.section({name: 'csgclaw:standing-instructions', order: 9400, interpolate: false,
    text: () => readFileSync(new URL('./standing-instructions.txt', import.meta.url), 'utf8')});
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
