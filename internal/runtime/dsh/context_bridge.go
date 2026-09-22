package dsh

// The native ACP adapter does not project compaction lifecycle events. This
// small managed plugin forwards those existing events without owning compaction.
const contextBridgeModule = `export const name = 'csgclaw-context';
export function apply(ctx) {
  ctx.on('agent/error', ({agent, error}) => {
    for (let e = error, depth = 0; e && depth < 8; e = e.cause, depth++) {
      if (e.code === 'CONTEXT_WINDOW_EXCEEDED') {
        process.stdout.write(JSON.stringify({jsonrpc:'2.0',method:'csgclaw/context',params:{sessionId:agent.session.id,update:{sessionUpdate:'context_error'}}}) + '\n');
        break;
      }
    }
  });
  ctx.on('session/event', (session, event) => {
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
