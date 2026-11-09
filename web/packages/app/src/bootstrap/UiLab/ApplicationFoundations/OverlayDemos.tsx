import { useState } from "react";
import {
  Button,
  Combobox,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
  DialogCloseButton,
  DialogTrigger,
  Field,
  TextInput,
} from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import styles from "./ApplicationFoundations.module.css";
export function ModalDemo({ drawer = false }: { drawer?: boolean }) {
  const { text: t } = useLabTranslation();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const [saved, setSaved] = useState("");
  const [notice, setNotice] = useState("");
  return (
    <>
      <DialogRoot open={open} onOpenChange={setOpen}>
        <DialogTrigger asChild>
          <Button variant="primary" onClick={() => setDraft(saved)}>
            {t(drawer ? "打开抽屉" : "打开弹窗")}
          </Button>
        </DialogTrigger>
        <DialogContent className={`${styles.demoSurface} ${drawer ? styles.drawer : styles.modal}`}>
          <DialogHeader>
            <div>
              <DialogTitle>{t(drawer ? "编辑设置" : "保存草稿")}</DialogTitle>
              <DialogDescription>{t("此处为本地交互示例，关闭不会提交修改。")}</DialogDescription>
            </div>
            <DialogCloseButton label={t("关闭")} />
          </DialogHeader>
          <DialogBody>
            <Field label={t("草稿名称")}>
              <TextInput value={draft} onChange={(e) => setDraft(e.target.value)} />
            </Field>
          </DialogBody>
          <DialogFooter>
            <Button onClick={() => setOpen(false)}>{t("取消")}</Button>
            <Button
              variant="primary"
              onClick={() => {
                setSaved(draft);
                setNotice("已保存本次草稿。");
                setOpen(false);
              }}
            >
              {t("保存")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
      <p role="status">
        {t(notice)} {saved}
      </p>
    </>
  );
}
export function CommandDemo() {
  const { text: t } = useLabTranslation();
  const [action, setAction] = useState("");
  return (
    <>
      <Combobox
        value={action}
        onValueChange={setAction}
        placeholder={t("选择操作")}
        searchPlaceholder={t("搜索命令")}
        emptyLabel={t("没有匹配的命令。")}
        triggerProps={{ "aria-label": t("命令菜单") }}
        options={["创建项目", "邀请成员", "查看资料", "帮助中心"].map((label) => ({ value: label, label: t(label) }))}
      />
      <p role="status">
        {action ? `${t("已选择命令")}：${t(action)}` : t("使用方向键选择命令，Enter 确认，Escape 关闭。")}
      </p>
    </>
  );
}
