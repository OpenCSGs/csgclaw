import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';

export const maxMemoryBytes = 32 * 1024;
export const maxHandbookBytes = 64 * 1024;
export const revision = content => crypto.createHash('sha256').update(content).digest('hex');

// All files belong to one Agent's managed home, outside its selected project.
export function createMemoryStore(home) {
  if (!home || !path.isAbsolute(home)) throw new Error('DSH_HOME must be absolute');
  const root = path.join(home, 'memories');
  const resolve = (name, create) => {
    const parts = name.split('/');
    if (parts.some(part => !/^[a-zA-Z0-9_.-]+$/.test(part) || part === '.' || part === '..')) throw new Error('Invalid memory path');
    let directory = root;
    for (const part of ['', ...parts.slice(0, -1)]) {
      if (part) directory = path.join(directory, part);
      if (create) fs.mkdirSync(directory, {recursive: true, mode: 0o700});
      const info = fs.lstatSync(directory, {throwIfNoEntry: false});
      if (!info) return;
      if (!info.isDirectory() || info.isSymbolicLink()) throw new Error('Memory directory must be a real directory');
    }
    return path.join(directory, parts.at(-1));
  };
  const readFile = (name, limit) => {
    const filename = resolve(name, false);
    if (!filename) return '';
    const info = fs.lstatSync(filename, {throwIfNoEntry: false});
    if (!info) return '';
    if (!info.isFile() || info.isSymbolicLink() || info.size > limit) throw new Error('Invalid memory file');
    const fd = fs.openSync(filename, fs.constants.O_RDONLY | (fs.constants.O_NOFOLLOW || 0));
    try {
      const buffer = Buffer.alloc(limit + 1);
      const size = fs.readSync(fd, buffer, 0, buffer.length, 0);
      if (size > limit) throw new Error('Memory file is too large');
      return buffer.subarray(0, size).toString('utf8');
    } finally { fs.closeSync(fd); }
  };
  const writeFile = (name, content, limit) => {
    if (typeof content !== 'string' || Buffer.byteLength(content, 'utf8') > limit) throw new Error('Memory content exceeds ' + limit + ' bytes');
    const filename = resolve(name, true);
    const info = fs.lstatSync(filename, {throwIfNoEntry: false});
    if (info && (!info.isFile() || info.isSymbolicLink())) throw new Error('Invalid memory file');
    const temporary = path.join(path.dirname(filename), '.memory-' + crypto.randomUUID());
    try {
      fs.writeFileSync(temporary, content, {encoding: 'utf8', mode: 0o600, flag: 'wx'});
      fs.renameSync(temporary, filename);
    } finally { fs.rmSync(temporary, {force: true}); }
  };
  const read = () => {
    const content = readFile('memory_summary.md', maxMemoryBytes);
    return {content, revision: revision(content)};
  };
  const readHandbook = () => {
    const content = readFile('MEMORY.md', maxHandbookBytes);
    return {content, revision: revision(content)};
  };
  const updateDocuments = (documents, expected) => {
    // Validate the entire response and both versions before replacing any file.
    for (const [content, limit] of [[documents.summary, maxMemoryBytes], [documents.handbook, maxHandbookBytes]]) {
      if (typeof content !== 'string' || Buffer.byteLength(content) > limit) throw new Error('Memory content exceeds ' + limit + ' bytes');
    }
    if (read().revision !== expected.summary || readHandbook().revision !== expected.handbook) throw new Error('Memory revision conflict; read and merge the current documents');
    writeFile('MEMORY.md', documents.handbook, maxHandbookBytes);
    writeFile('memory_summary.md', documents.summary, maxMemoryBytes);
    return read();
  };
  const update = (content, expectedRevision) => {
    if (typeof content !== 'string' || typeof expectedRevision !== 'string') throw new Error('Invalid memory update');
    if (read().revision !== expectedRevision) throw new Error('Memory revision conflict; read and merge the current summary');
    // An explicit replacement also replaces the current handbook. Otherwise a
    // forgotten fact could survive in retrieval despite disappearing from context.
    return updateDocuments({summary: content, handbook: content}, {summary: expectedRevision, handbook: readHandbook().revision});
  };
  const search = (query, limit = 5) => {
    if (typeof query !== 'string' || !query.trim() || Buffer.byteLength(query) > 512 || !Number.isInteger(limit) || limit < 1 || limit > 10) throw new Error('Invalid memory search');
    const terms = [...new Set(query.normalize('NFKC').toLowerCase().split(/\s+/).filter(Boolean))];
    const matches = [];
    for (const [document, text] of [['MEMORY.md', readHandbook().content], ['memory_summary.md', read().content]]) {
      for (const section of text.split(/(?=^#{1,3} )/m)) {
        const normalized = section.normalize('NFKC').toLowerCase();
        const score = terms.filter(term => normalized.includes(term)).length;
        if (!score) continue;
        const offset = Math.min(...terms.map(term => section.toLowerCase().indexOf(term)).filter(index => index >= 0));
        matches.push({document, score, content: section.slice(Math.max(0, Number.isFinite(offset) ? offset - 160 : 0), Math.max(0, Number.isFinite(offset) ? offset - 160 : 0) + 1600)});
      }
    }
    // Search current curated documents only; archived evidence may contain facts
    // subsequently corrected or forgotten and must not automatically return them.
    return matches.sort((a, b) => b.score - a.score).slice(0, limit).map(({score, ...match}) => match);
  };
  return {read, readHandbook, update, updateDocuments, search, readFile, writeFile};
}
