import messagePreviewTextStyles from "./MessagePreviewText.module.css";
import { Fragment } from "react";
import type { MessagePreviewTextToken } from "@/models/conversations";
import { splitMessagePreviewText } from "@/models/conversations";

export function MessagePreviewText({ content }: { content: unknown }) {
  const tokens: MessagePreviewTextToken[] = splitMessagePreviewText(content);

  if (tokens.length === 0) {
    return null;
  }

  return (
    <>
      {tokens.map((token, index) => {
        if (token.type === "mention") {
          return (
            <span
              key={`${token.text}-${index}`}
              className={`${messagePreviewTextStyles["message-preview-token"]} ${messagePreviewTextStyles["message-preview-token-mention"]}`}
            >
              {token.text}
            </span>
          );
        }
        if (token.type === "slash") {
          return (
            <span
              key={`${token.text}-${index}`}
              className={`${messagePreviewTextStyles["message-preview-token"]} ${messagePreviewTextStyles["message-preview-token-slash"]}`}
            >
              {token.text}
            </span>
          );
        }
        return <Fragment key={`${token.text}-${index}`}>{token.text}</Fragment>;
      })}
    </>
  );
}
