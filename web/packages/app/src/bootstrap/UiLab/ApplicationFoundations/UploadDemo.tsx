import { useEffect, useRef, useState, type CSSProperties } from "react";
import { Button, IconButton } from "@/components/ui";
import { LabOptions, LabOptionSelect } from "../LabOptions";
import { useLabTranslation } from "../LabLocale";
import { ThemeGlyph } from "../ThemeGlyph";
import { applicationAsset } from "./assets";
import layout from "./ApplicationFoundations.module.css";
import styles from "./UploadDemo.module.css";

const acceptedTypes = ["image/png", "image/jpeg", "image/gif", "image/svg+xml"];
const maxBytes = 2 * 1024 * 1024;

type QueueItem = { id: number; file: File; progress: number; rejected: boolean };

export function UploadDemo() {
  const { locale, text: t } = useLabTranslation();
  const zh = locale === "zh";
  const input = useRef<HTMLInputElement>(null);
  const nextID = useRef(0);
  const replacement = useRef<number | undefined>(undefined);
  const [queue, setQueue] = useState<QueueItem[]>([]);
  const [display, setDisplay] = useState("bar");
  const [breakpoint, setBreakpoint] = useState("desktop");
  const [disabled, setDisabled] = useState(false);
  const [dragging, setDragging] = useState(false);
  const pending = queue.some((item) => !item.rejected && item.progress < 100);

  useEffect(() => {
    if (!pending) return;
    const timer = window.setInterval(() => {
      setQueue((items) =>
        items.map((item) =>
          item.rejected || item.progress === 100 ? item : { ...item, progress: Math.min(100, item.progress + 20) },
        ),
      );
    }, 240);
    return () => window.clearInterval(timer);
  }, [pending]);

  function enqueue(files: FileList, replace?: number) {
    if (disabled || !files.length) return;
    const additions = Array.from(files, (file) => ({
      id: ++nextID.current,
      file,
      progress: 0,
      rejected: !acceptedTypes.includes(file.type) || file.size > maxBytes,
    }));
    setQueue((items) => [...items.filter((item) => item.id !== replace), ...additions]);
  }

  function choose(replace?: number) {
    replacement.current = replace;
    input.current?.click();
  }

  return (
    <div className={styles.demo}>
      <LabOptions>
        <LabOptionSelect
          label={t("设计断点")}
          value={breakpoint}
          onChange={setBreakpoint}
          options={[
            { value: "desktop", label: t("桌面端") },
            { value: "mobile", label: t("移动端") },
          ]}
        />
        <LabOptionSelect
          label={zh ? "进度样式" : "Progress style"}
          value={display}
          onChange={setDisplay}
          options={[
            { value: "bar", label: t("进度条") },
            { value: "fill", label: zh ? "背景填充" : "Background fill" },
          ]}
        />
        <LabOptionSelect
          label={zh ? "上传区状态" : "Upload area state"}
          value={disabled ? "disabled" : "enabled"}
          onChange={(value) => {
            setDisabled(value === "disabled");
            setDragging(false);
          }}
          options={[
            { value: "enabled", label: zh ? "可用" : "Enabled" },
            { value: "disabled", label: t("禁用") },
          ]}
        />
      </LabOptions>
      <div className={styles.preview} data-mobile={breakpoint === "mobile"}>
        <button
          type="button"
          className={styles.dropzone}
          data-dragging={dragging}
          disabled={disabled}
          onClick={() => choose()}
          onDragOver={(event) => {
            if (!event.dataTransfer.types.includes("Files")) return;
            event.preventDefault();
            event.dataTransfer.dropEffect = disabled ? "none" : "copy";
            if (!disabled) setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(event) => {
            event.preventDefault();
            setDragging(false);
            enqueue(event.dataTransfer.files);
          }}
        >
          <span className={styles.uploadIcon}>
            <img src={applicationAsset("file-upload-imgUploadCloud02.svg")} alt="" />
          </span>
          <span>
            <strong>{zh ? "点击上传" : "Click to upload"}</strong> {zh ? "或将文件拖到此处" : "or drag and drop"}
          </span>
          <small>{t("支持 PNG、JPG、GIF、SVG，最大 2 MB。仅本地校验，不上传服务器。")}</small>
        </button>
        <input
          ref={input}
          type="file"
          multiple
          disabled={disabled}
          accept={acceptedTypes.join(",")}
          tabIndex={-1}
          aria-label={t("选择文件")}
          className={layout.srOnly}
          onChange={(event) => {
            if (event.currentTarget.files) enqueue(event.currentTarget.files, replacement.current);
            event.currentTarget.value = "";
            replacement.current = undefined;
          }}
        />
        {queue.length > 0 && (
          <ul className={styles.queue} aria-label={zh ? "文件队列" : "File queue"}>
            {queue.map(({ id, file, progress, rejected }) => {
              const complete = progress === 100;
              const state = rejected ? "error" : complete ? "complete" : "progress";
              const status = rejected
                ? zh
                  ? "失败"
                  : "Failed"
                : complete
                  ? t("已完成")
                  : zh
                    ? "模拟上传中…"
                    : "Simulating upload…";
              return (
                <li
                  key={id}
                  className={styles.item}
                  data-state={state}
                  data-display={display}
                  style={{ "--upload-progress": `${progress}%` } as CSSProperties}
                >
                  <ThemeGlyph src={applicationAsset("file-upload-document.svg")} />
                  <div className={styles.details}>
                    <div className={styles.name} title={file.name}>
                      {file.name}
                    </div>
                    <div className={styles.metadata}>
                      <span>
                        {Math.round((file.size * progress) / 100)} / {file.size} B
                      </span>
                      <span className={styles.state}>
                        <img src={applicationAsset(`file-upload-${state}.svg`)} alt="" />
                        {status}
                      </span>
                      {!rejected && display === "fill" && <span>{progress}%</span>}
                    </div>
                    {rejected ? (
                      <>
                        <div className={styles.error} role="alert">
                          {t("文件类型不支持或超过 2 MB，请重新选择。")}
                        </div>
                        <Button size="sm" variant="linkGray" disabled={disabled} onClick={() => choose(id)}>
                          {zh ? "重新选择" : "Choose another file"}
                        </Button>
                      </>
                    ) : (
                      <div className={display === "bar" ? styles.progress : layout.srOnly}>
                        <progress value={progress} max={100} aria-label={file.name} />
                        <span>{progress}%</span>
                      </div>
                    )}
                  </div>
                  <IconButton
                    size="sm"
                    variant="linkGray"
                    label={`${t("移除文件")} ${file.name}`}
                    icon={<ThemeGlyph src={applicationAsset("file-upload-remove.svg")} />}
                    onClick={() => setQueue((items) => items.filter((item) => item.id !== id))}
                  />
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
