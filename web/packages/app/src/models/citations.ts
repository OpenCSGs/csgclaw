export type CitationSource = {
  id: string;
  title: string;
  snippet: string;
};

export type CitationDocument = {
  body: string;
  sources: Map<string, CitationSource>;
};

const numberedDefinitionPattern = /^\[([1-9]\d{0,5})\]:[ \t]*"(.+)"[ \t]*$/;
const numberedReferencePattern = /\(\[([^\]\n]{1,300})\]\[([1-9]\d{0,5})\]\)/g;
const fencePattern = /^ {0,3}(`{3,}|~{3,})/;

// Citation definitions are single-line Markdown footnotes. Keep fenced code intact.
export function parseCitationDocument(content: string): CitationDocument {
  const sources = new Map<string, CitationSource>();
  const body: string[] = [];
  const numberedDefinitions = new Map<string, string>();
  const numberedDefinitionLines = new Map<number, string>();
  let fence: { char: string; length: number } | null = null;

  for (const line of content.replace(/\r\n/g, "\n").split("\n")) {
    const fenceMatch = fencePattern.exec(line);
    if (fenceMatch) {
      const marker = fenceMatch[1];
      if (!fence) {
        fence = { char: marker[0], length: marker.length };
      } else if (marker[0] === fence.char && marker.length >= fence.length) {
        fence = null;
      }
      body.push(line);
      continue;
    }
    if (!fence) {
      const numbered = numberedDefinitionPattern.exec(line);
      if (numbered) {
        numberedDefinitions.set(numbered[1], numbered[2].trim());
        numberedDefinitionLines.set(body.length, numbered[1]);
        body.push(line);
        continue;
      }
    }
    body.push(line);
  }
  const usedNumbers = new Set<string>();
  fence = null;
  for (const line of body) {
    const fenceMatch = fencePattern.exec(line);
    if (fenceMatch) {
      const marker = fenceMatch[1];
      if (!fence) fence = { char: marker[0], length: marker.length };
      else if (marker[0] === fence.char && marker.length >= fence.length) fence = null;
      continue;
    }
    if (!fence && !numberedDefinitionPattern.test(line)) {
      for (const match of line.matchAll(numberedReferencePattern)) usedNumbers.add(match[2]);
    }
  }
  for (const id of usedNumbers) {
    const snippet = numberedDefinitions.get(id);
    if (snippet) sources.set(id, { id, title: "", snippet });
  }
  return {
    body: body
      .filter((_, index) => !sources.has(numberedDefinitionLines.get(index) ?? ""))
      .join("\n")
      .trimEnd(),
    sources,
  };
}
