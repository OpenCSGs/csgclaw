const memoryTools = new Set(['memory_read', 'memory_update', 'memory_search']);

const credentialKeys = new Set(['password', 'passwd', 'pwd', 'passphrase', 'apikey', 'accesskey', 'secretkey', 'secretaccesskey', 'accesstoken', 'refreshtoken', 'token', 'clientsecret', 'privatekey', 'authorization']);
const isCredentialKey = key => credentialKeys.has(key.normalize('NFKC').replace(/[\s_-]/g, '').toLowerCase());

function redactPlainText(text) {
  const secret = process.env.CSGCLAW_DSH_LLM_API_KEY;
  if (secret) text = text.split(secret).join('[REDACTED]');
  return text
    .replace(/-----BEGIN(?: [A-Z0-9]+)? PRIVATE KEY-----[\s\S]*?-----END(?: [A-Z0-9]+)? PRIVATE KEY-----/g, '[REDACTED]')
    .replace(/\b(?:sk-[\w-]{16,}|glpat-[\w-]+|gh[pousr]_[\w]+|github_pat_[\w]+|AKIA[A-Z0-9]{16}|xox[baprs]-[\w-]+)\b/g, '[REDACTED]')
    .replace(/\beyJ[\w-]{10,}\.[\w-]+\.[\w-]+\b/g, '[REDACTED]')
    .replace(/\bBearer\s+[\w.+/=-]+/gi, 'Bearer [REDACTED]')
    .replace(/((?:api[_ -]?key|access[_ -]?token|refresh[_ -]?token|password|client[_ -]?secret)\s*[=:]\s*)(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s,"';]+)/gi, '$1[REDACTED]');
}

// Parse JSON before redaction so quoted keys, escaped values and nested JSON
// strings are handled without damaging their surrounding JSON representation.
export function redactMemory(text, depth = 0) {
  if (depth >= 16) return '[REDACTED]';
  let parsed, structured = false;
  try {
    parsed = JSON.parse(text);
    structured = parsed !== null && (typeof parsed === 'object' || typeof parsed === 'string');
  } catch {}
  if (structured) {
    const visit = (value, level) => {
      if (level >= 16) return '[REDACTED]';
      if (typeof value === 'string') return redactMemory(value, level + 1);
      if (Array.isArray(value)) return value.map(item => visit(item, level + 1));
      if (value !== null && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, isCredentialKey(key) ? '[REDACTED]' : visit(item, level + 1)]));
      return value;
    };
    return JSON.stringify(visit(parsed, depth));
  }
  // Tool output can contain JSON embedded in prose rather than one whole JSON
  // document. Match complete quoted strings (including escapes), never a prefix
  // of a credential value. Nested objects are inspected via their own keys.
  const scanned = text;
  let unmatched = '', position = 0;
  text = scanned.replace(/"(?:\\.|[^"\\])*"/g, (literal, offset) => {
    try {
      const value = JSON.parse(literal);
      unmatched += scanned.slice(position, offset);
      position = offset + literal.length;
      const sanitized = redactMemory(value, depth + 1);
      return sanitized === value ? literal : JSON.stringify(sanitized);
    } catch { return literal; }
  });
  unmatched += scanned.slice(position);
  text = text.replace(/("(?:\\.|[^"\\])*")(\s*:\s*)("(?:\\.|[^"\\])*"|[^\s,\[\]{}"]+)/g, (match, key, separator, value) => {
    try {
      if (isCredentialKey(JSON.parse(key))) return key + separator + '"[REDACTED]"';
      if (value.startsWith('"')) return key + separator + JSON.stringify(redactMemory(JSON.parse(value), depth + 1));
    } catch {}
    return match;
  });
  // Unwrapped escaped JSON is not a valid string/document to decode. Do not
  // guess credential-value boundaries and accidentally retain their suffixes.
  if (/\\+"(?:password|api[_ -]?key|access[_ -]?token|refresh[_ -]?token|client[_ -]?secret)\\+"\s*[:=]/i.test(unmatched)) return '[REDACTED]';
  return redactPlainText(text);
}

function tail(text, bytes) {
  const buffer = Buffer.from(text);
  if (buffer.length <= bytes) return text;
  let start = buffer.length - bytes;
  while ((buffer[start] & 0xc0) === 0x80) start++;
  return buffer.subarray(start).toString();
}

function bounded(text, bytes) {
  text = redactMemory(text);
  const buffer = Buffer.from(text);
  if (buffer.length <= bytes) return text;
  let end = Math.floor(bytes / 2) - 32;
  while ((buffer[end] & 0xc0) === 0x80) end--;
  return buffer.subarray(0, end).toString() + '\n[truncated]\n' + tail(text, Math.floor(bytes / 2));
}

function textOf(blocks) {
  return (blocks ?? []).filter(block => block.type === 'text').map(block => block.text).join('\n');
}

// Only the fixed DSH public session event API is used; no raw request headers,
// system prompts, reasoning streams, tool-private metadata or injected messages.
export function captureConversation(session, previousCursor, maxBytes) {
  if (typeof session.snapshotEvents !== 'function') {
    const messages = session.deriveMessages().filter(message => message.role === 'assistant' || (message.role === 'user' && message.source.kind === 'user'));
    const cursor = previousCursor ? messages.findIndex(message => message.id === previousCursor) : -1;
    const fresh = messages.slice(cursor + 1);
    if (!fresh.some(message => message.role === 'user')) return;
    return {cursor: messages.at(-1).id, transcript: tail(fresh.map(message => message.role + ': ' + bounded(textOf(message.content), 4096)).join('\n\n'), maxBytes)};
  }
  const events = session.snapshotEvents();
  const eventCursor = /^event:(\d+)$/.exec(previousCursor ?? '');
  const after = eventCursor ? Number(eventCursor[1]) : previousCursor ? events.find(event => (event.data?.message ?? event.data)?.id === previousCursor)?.seq ?? -1 : -1;
  const lines = [], calls = new Map();
  let directTurn = false, hasUser = false;
  for (const event of events) {
    const data = event.data;
    if (event.type === 'turn/start') directTurn = false;
    if (event.type === 'user/message') {
      if (data.source?.kind === 'user') directTurn = true;
      if (event.seq > after && data.source?.kind === 'user') {
        hasUser = true;
        lines.push('[event ' + event.seq + '] user: ' + bounded(textOf(data.content), 4096));
      }
      continue;
    }
    if (!directTurn) continue;
    if (event.type === 'tool/call') calls.set(data.callId, data.name);
    if (event.seq <= after) continue;
    const prefix = '[event ' + event.seq + '] ';
    if (event.type === 'assistant/message') {
      const text = textOf(data.message?.content);
      if (text.trim()) lines.push(prefix + 'assistant' + (data.interrupted ? ' (interrupted)' : '') + ': ' + bounded(text, 4096));
    } else if (event.type === 'tool/call' && !memoryTools.has(data.name)) {
      lines.push(prefix + 'tool call ' + data.name + ': ' + bounded(data.arguments, 2048));
    } else if (event.type === 'tool/result') {
      for (const block of data.message?.content ?? []) {
        if (block.type !== 'tool-result') continue;
        const name = calls.get(block.toolCallId);
        // Unknown or memory-owned results must not become independent evidence.
        if (!name || memoryTools.has(name)) continue;
        lines.push(prefix + 'tool result ' + name + ' (' + (block.isError ? 'error' : 'returned; inspect output for verification') + '): ' + bounded(textOf(block.content), 4096));
      }
    } else if (event.type === 'turn/end') {
      lines.push(prefix + 'turn ended: ' + data.reason.kind + (data.reason.kind === 'error' ? ' ' + bounded(JSON.stringify(data.reason.error ?? {}), 1024) : ''));
    }
  }
  if (!hasUser || !events.length) return;
  // Retain the latest direct user instruction even when many tool results fill
  // the byte budget. Each output is already independently bounded and redacted.
  let transcript = tail(redactMemory(lines.join('\n\n')), maxBytes);
  const lastUser = lines.findLast(line => /\] user: /.test(line));
  if (lastUser && !transcript.includes(lastUser)) transcript = lastUser + '\n\n[earlier evidence truncated]\n' + tail(transcript, maxBytes - Buffer.byteLength(lastUser) - 64);
  return {cursor: 'event:' + events.at(-1).seq, transcript};
}
