import { Fragment } from "react";
import type { MessagePreviewTextToken } from "@/models/conversations";
import { splitMessagePreviewText } from "@/models/conversations";
import "./MessagePreviewText.css";

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
            <span key={`${token.text}-${index}`} className="message-preview-token message-preview-token-mention">
              {token.text}
            </span>
          );
        }
        if (token.type === "slash") {
          return (
            <span key={`${token.text}-${index}`} className="message-preview-token message-preview-token-slash">
              {token.text}
            </span>
          );
        }
        return <Fragment key={`${token.text}-${index}`}>{token.text}</Fragment>;
      })}
    </>
  );
}
