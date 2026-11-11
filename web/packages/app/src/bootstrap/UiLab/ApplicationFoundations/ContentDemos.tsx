import { applicationAsset } from "./assets";
import { useRef, useState, type ChangeEvent } from "react";
import { Button, Field, TextArea } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import styles from "./ApplicationFoundations.module.css";
export function MessagingDemo() {
  const { text: t } = useLabTranslation();
  const [draft, setDraft] = useState("");
  const [messages, setMessages] = useState<string[]>([]);
  const [error, setError] = useState(false);
  return (
    <>
      <ol className={styles.messages}>
        <li>
          <strong>Phoenix Baker</strong>
          <p>{t("请帮忙查看最新设计。")}</p>
        </li>
        {messages.map((text, i) => (
          <li className={styles.sent} key={i}>
            <strong>{t("我")}</strong>
            <p>{text}</p>
            <small>{t("已在本地发送")}</small>
          </li>
        ))}
      </ol>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (!draft.trim()) {
            setError(true);
            return;
          }
          setMessages([...messages, draft.trim()]);
          setDraft("");
          setError(false);
        }}
      >
        <Field label={t("消息内容")} error={error ? t("请输入消息内容。") : undefined}>
          <TextArea
            value={draft}
            onChange={(event) => {
              setDraft(event.target.value);
              setError(false);
            }}
          />
        </Field>
        <Button type="submit" variant="primary">
          {t("发送消息")}
        </Button>
      </form>
      <p role="status">
        {t("已发送")} {messages.length}
      </p>
    </>
  );
}
export function UploadDemo() {
  const { text: t } = useLabTranslation();
  const input = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState(false);
  function choose(event: ChangeEvent<HTMLInputElement>) {
    const picked = event.target.files?.[0];
    event.target.value = "";
    if (!picked) return;
    const valid =
      ["image/png", "image/jpeg", "image/gif", "image/svg+xml"].includes(picked.type) && picked.size <= 2 * 1024 * 1024;
    setError(!valid);
    setFile(valid ? picked : null);
  }
  return (
    <>
      <div className={styles.upload}>
        <img src={applicationAsset("file-upload-imgUploadCloud02.svg")} alt="" width="20" height="20" />
        <Button variant="linkColor" onClick={() => input.current?.click()}>
          {t("选择文件")}
        </Button>
        <p>{t("支持 PNG、JPG、GIF、SVG，最大 2 MB。仅本地校验，不上传服务器。")}</p>
        <input
          ref={input}
          type="file"
          accept="image/png,image/jpeg,image/gif,image/svg+xml"
          aria-label={t("选择文件")}
          onChange={choose}
          className={styles.srOnly}
        />
      </div>
      {error && <p role="alert">{t("文件类型不支持或超过 2 MB，请重新选择。")}</p>}
      {file && (
        <div>
          <p role="status">
            {file.name} · {file.size} B · {t("本地校验通过")}
          </p>
          <Button onClick={() => setFile(null)}>{t("移除文件")}</Button>
        </div>
      )}
    </>
  );
}
const example = 'export const theme = {\n  fontFamily: "PingFang SC",\n  primary: "#3250bd",\n};';
export function CodeDemo() {
  const { text: t } = useLabTranslation();
  const [notice, setNotice] = useState("");
  return (
    <>
      <pre className={styles.code}>
        <code>{example}</code>
      </pre>
      <Button
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(example);
            setNotice("代码已复制。");
          } catch {
            setNotice("复制失败，请选择代码后手动复制。");
          }
        }}
      >
        {t("复制代码")}
      </Button>
      <p role="status">{t(notice)}</p>
    </>
  );
}
