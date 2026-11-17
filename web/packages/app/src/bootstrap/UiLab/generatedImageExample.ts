import type { IMMessage } from "@/models/conversations";

export function generatedImageExample(zh: boolean, createdAt: string, senderID?: string): IMMessage {
  const previewURL = "avatar/cartoon-5.png";
  return {
    id: "ui-lab-generated-image",
    sender_id: senderID,
    created_at: createdAt,
    content: zh ? "生成一张卡通头像。" : "Generate a cartoon avatar.",
    metadata: { image_generation: { state: "completed" } },
    attachments: [
      {
        id: "ui-lab-avatar",
        created_at: createdAt,
        kind: "image",
        size_bytes: 43793,
        sha256: "95e43f2a2221d8e11437257fb03e19e3b87dfd885c7c64aa6db2ffb20b2ecda7",
        name: zh ? "卡通头像.png" : "Cartoon avatar.png",
        media_type: "image/png",
        preview_url: previewURL,
        download_url: previewURL,
      },
    ],
  };
}
