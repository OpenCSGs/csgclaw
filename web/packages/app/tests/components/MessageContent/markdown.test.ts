import { createElement } from "react";
import { render, screen } from "@testing-library/react";
import { decorateMentionMarkup, escapeHTML, flattenMentionText } from "@/components/business/MessageContent/mentions";
import { renderMarkdown } from "@/components/business/MessageContent/markdown";

describe("message markdown helpers", () => {
  it("escapes and decorates mention markup", () => {
    expect(escapeHTML('<script>"x"</script>')).toBe("&lt;script&gt;&quot;x&quot;&lt;/script&gt;");
    expect(flattenMentionText('Hi <at user_id="u-1">Alice</at>')).toBe("Hi @Alice");
    expect(decorateMentionMarkup('Hi <at user_id="<bad>">A&B</at>')).toBe(
      'Hi <span class="message-mention" data-user-id="&lt;bad&gt;">@A&amp;B</span>',
    );
  });

  it("renders basic markdown and strips unsafe HTML", () => {
    const html = renderMarkdown(
      ["Hello **world**", '<img src=x onerror="alert(1)">', '<script>alert("xss")</script>'].join("\n"),
    );

    expect(html).toContain("<strong>world</strong>");
    expect(html).not.toContain("<script");
    expect(html).not.toContain("onerror");
  });

  it("adds safe attributes to links after sanitization", () => {
    render(
      createElement("div", {
        dangerouslySetInnerHTML: { __html: renderMarkdown("[docs](https://example.com/docs)") },
      }),
    );
    const link = screen.getByRole("link", { name: "docs" });

    expect(link).toHaveAttribute("href", "https://example.com/docs");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("preserves structured resource link icons", () => {
    render(
      createElement("div", {
        dangerouslySetInnerHTML: {
          __html: renderMarkdown(
            '- <img class="resource-link-icon" src="https://example.com/icon.svg" alt="" aria-hidden="true" onerror="alert(1)"> [docs](<https://example.com/docs>)',
          ),
        },
      }),
    );

    const icon = document.querySelector("img.resource-link-icon");
    expect(icon).toHaveAttribute("src", "https://example.com/icon.svg");
    expect(icon).toHaveAttribute("alt", "");
    expect(icon).toHaveAttribute("aria-hidden", "true");
    expect(icon).not.toHaveAttribute("onerror");

    const link = screen.getByRole("link", { name: "docs" });
    expect(link).toHaveAttribute("href", "https://example.com/docs");
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("renders decorated mentions through the markdown pipeline", () => {
    const html = renderMarkdown('Ping <at user_id="u-1">Alice</at>');

    expect(html).toContain('class="message-mention"');
    expect(html).toContain('data-user-id="u-1"');
    expect(html).toContain("@Alice");
  });
});
