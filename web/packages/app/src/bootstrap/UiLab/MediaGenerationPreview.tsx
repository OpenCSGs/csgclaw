import { useState } from "react";
import { Button, MediaGeneration } from "@/components/ui";
import { MessageAttachments } from "@/components/business/ConversationPane/ConversationAttachments";
import { ImageGenerationStatus } from "@/components/business/ConversationPane/ImageGenerationStatus";
import { DocumentPreviewPanel, type DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import { generatedImageExample } from "./generatedImageExample";
import { createTranslator } from "@/shared/i18n";
import { InteractiveExample } from "./ApplicationFoundations/InteractiveExample";
import { useLabTranslation } from "./LabLocale";

export function MediaGenerationPreview() {
  const { locale } = useLabTranslation();
  const zh = locale === "zh";

  return (
    <InteractiveExample
      id="media-generation"
      title={zh ? "媒体生成" : "Media generation"}
      description={
        zh
          ? "预览排队、生成、完成和失败状态。可选择生成版本，或重试失败的任务。"
          : "Preview queued, generating, completed, and failed states. Select a variation or retry the failed task."
      }
    >
      <MediaGenerationDemo />
    </InteractiveExample>
  );
}

function MediaGenerationDemo() {
  const [retried, setRetried] = useState(false);
  const [preview, setPreview] = useState<DocumentPreviewRequest>();
  const [createdAt] = useState(() => new Date().toISOString());
  const [variation, setVariation] = useState<number>();
  const { locale } = useLabTranslation();
  const t = createTranslator(locale);
  const zh = locale === "zh";
  const generatedImage = generatedImageExample(zh, createdAt);
  const prompt = zh
    ? "状态页主图：黎明时分安静的控制室，光线柔和，无人。"
    : "Hero image for the Northwind status page: a calm control room at dawn, soft light, no people.";

  return (
    <div className="grid gap-6">
      <ImageGenerationStatus
        message={generatedImage}
        t={t}
        result={
          <MessageAttachments
            generation
            attachments={generatedImage.attachments}
            t={t}
            onPreviewAttachment={setPreview}
          />
        }
      />
      {preview ? (
        <DocumentPreviewPanel
          {...preview}
          t={t}
          onClose={() => setPreview(undefined)}
          onIndexChange={(index) => setPreview((current) => current && { ...current, index })}
        />
      ) : null}
      <MediaGeneration kind="video" state="queued" label={t("videoQueued")} prompt={prompt} />
      <MediaGeneration
        kind="image"
        state="pending"
        aspectRatio={16 / 9}
        label={zh ? "正在生成 · 约剩 9 秒" : "Generating · about 9s left"}
        progress={42}
        prompt={prompt}
      />
      <MediaGeneration
        kind="image"
        state="completed"
        label={zh ? "已完成 · 4 个版本" : "Done · 4 variations"}
        prompt={
          zh
            ? "为状态页设计四组扁平的单色图标。"
            : "Four icon options for the Northwind status page, flat, single colour."
        }
        variations={Array.from({ length: 4 }, (_, index) => ({
          label: zh ? `版本 ${index + 1}` : `Variation ${index + 1}`,
          onSelect: () => setVariation(index + 1),
        }))}
      />
      {variation ? (
        <span role="status">{zh ? `已选择版本 ${variation}` : `Selected variation ${variation}`}</span>
      ) : null}
      <MediaGeneration
        kind="image"
        state={retried ? "pending" : "failed"}
        aspectRatio={4 / 3}
        label={retried ? t("imageGenerating") : zh ? "生成失败" : "Generation failed"}
        actions={
          <Button size="sm" variant="secondaryGray" onClick={() => setRetried(true)}>
            {zh ? "重试" : "Retry"}
          </Button>
        }
        failureMessage={
          zh
            ? "图片服务在 60 秒后超时。本次未扣费。"
            : "The image service timed out after 60 seconds. Nothing was charged."
        }
        prompt={prompt}
      />
      <MediaGeneration
        kind="image"
        state="pending"
        aspectRatio={9 / 16}
        progress={68}
        label={zh ? "正在生成 · 约剩 4 秒" : "Generating · about 4s left"}
        prompt={zh ? "产品发布图的竖版故事尺寸。" : "Story-sized version of the launch visual for Lumen Labs."}
      />
    </div>
  );
}
