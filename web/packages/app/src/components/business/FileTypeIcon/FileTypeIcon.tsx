import styles from "./FileTypeIcon.module.css";

const assets = import.meta.glob<string>("/src/assets/file-types/*.svg", {
  eager: true,
  import: "default",
  query: "?url",
});

type FileIcon = { page: string; label?: string; mark?: string; wideMark?: boolean };

const formatIcons: Record<string, FileIcon> = {
  pdf: { page: "pdf", label: "PDF" },
  doc: { page: "doc", label: "DOC" },
  docx: { page: "docx", label: "DOCX" },
  md: { page: "md", label: "MD" },
  txt: { page: "txt", label: "TXT" },
  csv: { page: "csv", label: "CSV" },
  xls: { page: "xls", label: "XLS" },
  xlsx: { page: "xlsx", label: "XLSX" },
  ppt: { page: "ppt", label: "PPT" },
  pptx: { page: "pptx", label: "PPTX" },
  ai: { page: "ai", label: "AI" },
  psd: { page: "psd", label: "PSD" },
  indd: { page: "indd", label: "INDD" },
  aep: { page: "aep", label: "AEP" },
  mp3: { page: "mp3", label: "MP3" },
  wav: { page: "wav", label: "WAV" },
  mp4: { page: "mp4", label: "MP4" },
  mpeg: { page: "mpeg", label: "MPEG" },
  mpg: { page: "mpeg", label: "MPEG" },
  html: { page: "html", label: "HTML" },
  htm: { page: "html", label: "HTML" },
  css: { page: "css", label: "CSS" },
  rss: { page: "rss", label: "RSS" },
  sql: { page: "sql", label: "SQL" },
  js: { page: "js", label: "JS" },
  mjs: { page: "js", label: "JS" },
  cjs: { page: "js", label: "JS" },
  json: { page: "json", label: "JSON" },
  java: { page: "java", label: "JAVA" },
  xml: { page: "xml", label: "XML" },
  exe: { page: "exe", label: "EXE" },
  zip: { page: "zip", label: "ZIP" },
  rar: { page: "rar", label: "RAR" },
  dmg: { page: "dmg", label: "DMG" },
  mkv: { page: "mkv", label: "MKV" },
  avi: { page: "avi", label: "AVI" },
  fig: { page: "fig", label: "FIG" },
  svg: { page: "fig", label: "SVG" },
  gif: { page: "fig", label: "GIF" },
  tif: { page: "fig", label: "TIFF" },
  tiff: { page: "fig", label: "TIFF" },
  webp: { page: "fig", label: "WEBP" },
  png: { page: "fig", label: "PNG" },
  jpeg: { page: "fig", label: "JPEG" },
  jpg: { page: "fig", label: "JPG" },
};

const defaultIcons: Record<string, FileIcon> = {
  code: { page: "html", mark: "default-code-mark" },
  spreadsheet: { page: "xlsx", mark: "default-spreadsheet-mark", wideMark: true },
  document: { page: "docx", mark: "default-document-mark", wideMark: true },
  image: { page: "fig", mark: "default-image-mark" },
  video: { page: "mp4", mark: "default-video-mark" },
  audio: { page: "mp3", mark: "default-audio-mark" },
  empty: { page: "fig" },
};

const codeExtensions = new Set([
  "py",
  "go",
  "rs",
  "c",
  "h",
  "cc",
  "cpp",
  "hpp",
  "ts",
  "tsx",
  "jsx",
  "vue",
  "sh",
  "bash",
  "zsh",
  "yaml",
  "yml",
  "toml",
  "ini",
  "conf",
  "env",
  "log",
]);
const spreadsheetExtensions = new Set(["ods", "numbers", "xlt", "xltx"]);
const documentExtensions = new Set(["rtf", "odt", "pages"]);
const videoExtensions = new Set(["mov", "m4v", "webm", "3gp", "wmv", "flv"]);
const audioExtensions = new Set(["aac", "ogg", "oga", "flac", "m4a", "wma", "opus"]);

function getDefaultType(extension: string, mediaType: string) {
  if (codeExtensions.has(extension)) return "code";
  if (spreadsheetExtensions.has(extension)) return "spreadsheet";
  if (documentExtensions.has(extension)) return "document";
  if (videoExtensions.has(extension) || mediaType.startsWith("video/")) return "video";
  if (audioExtensions.has(extension) || mediaType.startsWith("audio/")) return "audio";
  if (mediaType.startsWith("image/")) return "image";
  if (/spreadsheet|excel/.test(mediaType)) return "spreadsheet";
  if (/word|document|pdf/.test(mediaType)) return "document";
  if (/javascript|typescript|json|xml|sql/.test(mediaType)) return "code";
  return "empty";
}

export function FileTypeIcon({
  name = "",
  mediaType = "",
  isFolder = false,
}: {
  name?: string;
  mediaType?: string;
  isFolder?: boolean;
}) {
  if (isFolder) return <span className={`${styles.icon} ${styles.folder}`} aria-hidden="true" />;

  const filename = name.replaceAll("\\", "/").split("/").pop() ?? "";
  const extension = filename.includes(".") ? filename.slice(filename.lastIndexOf(".") + 1).toLowerCase() : "";
  const icon =
    formatIcons[extension] ??
    (assets[`/src/assets/file-types/${extension}.svg`]
      ? { page: extension, label: extension.toUpperCase() }
      : defaultIcons[getDefaultType(extension, mediaType.toLowerCase())]);
  const page = assets[`/src/assets/file-types/${icon.page}.svg`];
  const mark = icon.mark && assets[`/src/assets/file-types/${icon.mark}.svg`];

  return (
    <span className={styles.icon} aria-hidden="true">
      {page ? <img src={page} width={32} height={40} alt="" /> : null}
      {icon.label ? <span>{icon.label}</span> : null}
      {mark ? (
        <span className={`${styles.mark}${icon.wideMark ? ` ${styles.wideMark}` : ""}`}>
          <img src={mark} width={icon.wideMark ? 15.9 : 16} height={icon.wideMark ? 11.1 : 16} alt="" />
        </span>
      ) : null}
    </span>
  );
}
