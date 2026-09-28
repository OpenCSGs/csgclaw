import type { DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import type { IMMessage, TranslateFn } from "@/models/conversations";
import { MessageAttachments } from "./ConversationAttachments";
import { VideoGenerationStatus } from "./VideoGenerationStatus";

export function VideoGenerationCard({
  message,
  onPreviewAttachment,
  t,
}: {
  message: IMMessage;
  onPreviewAttachment?: (request: DocumentPreviewRequest) => void;
  t: TranslateFn;
}) {
  return (
    <div className="video-generation-card">
      <VideoGenerationStatus message={message} t={t} />
      <MessageAttachments attachments={message.attachments} t={t} onPreviewAttachment={onPreviewAttachment} />
    </div>
  );
}
