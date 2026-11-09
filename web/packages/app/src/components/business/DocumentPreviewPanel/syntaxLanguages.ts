export const syntaxLanguages = {
  bash: () => import("@shikijs/langs/bash"),
  c: () => import("@shikijs/langs/c"),
  cpp: () => import("@shikijs/langs/cpp"),
  css: () => import("@shikijs/langs/css"),
  csv: () => import("@shikijs/langs/csv"),
  diff: () => import("@shikijs/langs/diff"),
  dockerfile: () => import("@shikijs/langs/dockerfile"),
  dotenv: () => import("@shikijs/langs/dotenv"),
  fish: () => import("@shikijs/langs/fish"),
  go: () => import("@shikijs/langs/go"),
  graphql: () => import("@shikijs/langs/graphql"),
  hcl: () => import("@shikijs/langs/hcl"),
  html: () => import("@shikijs/langs/html"),
  ini: () => import("@shikijs/langs/ini"),
  java: () => import("@shikijs/langs/java"),
  javascript: () => import("@shikijs/langs/javascript"),
  json: () => import("@shikijs/langs/json"),
  jsonc: () => import("@shikijs/langs/jsonc"),
  jsonl: () => import("@shikijs/langs/jsonl"),
  jsx: () => import("@shikijs/langs/jsx"),
  kotlin: () => import("@shikijs/langs/kotlin"),
  less: () => import("@shikijs/langs/less"),
  log: () => import("@shikijs/langs/log"),
  lua: () => import("@shikijs/langs/lua"),
  makefile: () => import("@shikijs/langs/makefile"),
  markdown: () => import("@shikijs/langs/markdown"),
  php: () => import("@shikijs/langs/php"),
  properties: () => import("@shikijs/langs/properties"),
  protobuf: () => import("@shikijs/langs/protobuf"),
  python: () => import("@shikijs/langs/python"),
  ruby: () => import("@shikijs/langs/ruby"),
  rust: () => import("@shikijs/langs/rust"),
  scss: () => import("@shikijs/langs/scss"),
  shellscript: () => import("@shikijs/langs/shellscript"),
  sql: () => import("@shikijs/langs/sql"),
  terraform: () => import("@shikijs/langs/terraform"),
  toml: () => import("@shikijs/langs/toml"),
  tsx: () => import("@shikijs/langs/tsx"),
  typescript: () => import("@shikijs/langs/typescript"),
  xml: () => import("@shikijs/langs/xml"),
  yaml: () => import("@shikijs/langs/yaml"),
  zsh: () => import("@shikijs/langs/zsh"),
} as const;

export type SyntaxLanguage = keyof typeof syntaxLanguages;

const extensionLanguages: Record<string, SyntaxLanguage> = {
  bash: "bash",
  c: "c",
  cc: "cpp",
  conf: "ini",
  cpp: "cpp",
  css: "css",
  csv: "csv",
  diff: "diff",
  env: "dotenv",
  fish: "fish",
  go: "go",
  gql: "graphql",
  graphql: "graphql",
  h: "c",
  hcl: "hcl",
  hpp: "cpp",
  htm: "html",
  html: "html",
  ini: "ini",
  java: "java",
  js: "javascript",
  json: "json",
  jsonc: "jsonc",
  jsonl: "jsonl",
  jsx: "jsx",
  kt: "kotlin",
  kts: "kotlin",
  less: "less",
  log: "log",
  lua: "lua",
  md: "markdown",
  markdown: "markdown",
  mjs: "javascript",
  mts: "typescript",
  patch: "diff",
  php: "php",
  properties: "properties",
  proto: "protobuf",
  py: "python",
  rb: "ruby",
  rs: "rust",
  scss: "scss",
  sh: "shellscript",
  sql: "sql",
  tf: "terraform",
  tfvars: "terraform",
  toml: "toml",
  ts: "typescript",
  tsx: "tsx",
  xml: "xml",
  yaml: "yaml",
  yml: "yaml",
  zsh: "zsh",
};

const basenameLanguages: Record<string, SyntaxLanguage> = {
  ".env": "dotenv",
  containerfile: "dockerfile",
  dockerfile: "dockerfile",
  gnumakefile: "makefile",
  makefile: "makefile",
};

const mediaTypeLanguages: Record<string, SyntaxLanguage> = {
  "application/json": "json",
  "application/toml": "toml",
  "application/x-ndjson": "jsonl",
  "application/x-yaml": "yaml",
  "application/yaml": "yaml",
  "text/css": "css",
  "text/csv": "csv",
  "text/html": "html",
  "text/markdown": "markdown",
  "text/x-go": "go",
  "text/x-python": "python",
  "text/x-rust": "rust",
  "text/yaml": "yaml",
};

export function syntaxLanguageForFile(name: string, mediaType: string): SyntaxLanguage | null {
  const normalizedName = name.trim().toLowerCase();
  const basename = normalizedName.split(/[\\/]/).pop() || normalizedName;
  const byBasename = basenameLanguages[basename];
  if (byBasename) {
    return byBasename;
  }
  const extensionStart = basename.lastIndexOf(".");
  if (extensionStart >= 0 && extensionStart < basename.length - 1) {
    const byExtension = extensionLanguages[basename.slice(extensionStart + 1)];
    if (byExtension) {
      return byExtension;
    }
  }
  const normalizedMediaType = mediaType.toLowerCase().split(";", 1)[0]?.trim() ?? "";
  return mediaTypeLanguages[normalizedMediaType] ?? null;
}
