// @vitest-environment jsdom

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CitationSources } from "./CitationSources";
import { MessageContent } from "./MessageContent";
import { renderMarkdownWithCitations } from "./markdown";
import { CONVERSATION_CITATION_PANEL_WIDTH_STORAGE_KEY } from "@/shared/storage/keys";

const numberedAnswer = `文昌鸡值得一试 ([海南美食指南.pdf][5])，也可参考这份文档 ([海南美食指南.pdf][5])。

[5]: "文昌鸡是海南特色美食。"`;

describe("answer citations", () => {
  it("leaves unmatched markers as text", () => {
    const rendered = renderMarkdownWithCitations("没有来源 ([缺失.pdf][7])");
    expect(rendered.cited).toHaveLength(0);
    expect(rendered.html).toContain("([缺失.pdf][7])");
  });

  it("does not treat an unquoted definition as a citation", () => {
    const rendered = renderMarkdownWithCitations("地址 ([初始房源信息.xlsx][1])。\n\n[1]: 无引号片段");
    expect(rendered.cited).toHaveLength(0);
    expect(rendered.html).not.toContain("data-citation-id");
  });

  it("renders the document name while keeping the citation number internal", () => {
    const rendered = renderMarkdownWithCitations(numberedAnswer);
    expect(rendered.cited).toEqual([
      { id: "5", title: "海南美食指南.pdf", snippet: "文昌鸡是海南特色美食。", number: 5 },
    ]);
    expect(rendered.html.match(/data-citation-id="5"/g)).toHaveLength(2);
    expect(rendered.html).not.toContain("[5]:");
    expect(rendered.html).toContain(">海南美食指南.pdf</button>");
    expect(rendered.html).not.toContain(">[海南美食指南.pdf]</button>");
    expect(rendered.html).not.toContain(">5</button>");
  });

  it("shows the full spreadsheet filename instead of citation numbers", () => {
    const rendered = renderMarkdownWithCitations(
      `地址甲 ([初始房源信息.xlsx][1])，地址乙 ([初始房源信息.xlsx][2])。\n\n[1]: "甲片段"\n[2]: "乙片段"`,
    );
    expect(rendered.html.match(/>初始房源信息\.xlsx<\/button>/g)).toHaveLength(2);
    expect(rendered.cited.map((item) => item.number)).toEqual([1, 2]);
    expect(rendered.cited.map((item) => item.snippet)).toEqual(["甲片段", "乙片段"]);
  });

  it("keeps the evidence in the definition, not the inline document link", () => {
    const snippet = "韩资园二期15#厂房，面积4420.62，框架及门式钢架结构，配备吊车梁：有";
    const rendered = renderMarkdownWithCitations(`地址 ([初始房源信息.xlsx][1])。\n\n[1]: "${snippet}"`);
    expect(rendered.html).toContain(">初始房源信息.xlsx</button>");
    expect(rendered.html).not.toContain(snippet);
    expect(rendered.html).not.toContain("[1]:");
    expect(rendered.cited[0].snippet).toBe(snippet);
  });

  it("does not convert numbered examples inside code fences", () => {
    const rendered = renderMarkdownWithCitations(
      `${numberedAnswer}\n\n\`\`\`md\n([example.pdf][5])\n[5]: "代码中的片段"\n\`\`\``,
    );
    expect(rendered.html).toContain("([example.pdf][5])");
    expect(rendered.html).toContain('[5]: "代码中的片段"');
  });

  it("opens the matching document and snippet only on click", async () => {
    render(<MessageContent content={numberedAnswer} t={(key) => key} />);
    const links = screen.getAllByRole("button", { name: "查看引用：海南美食指南.pdf" });
    expect(links).toHaveLength(2);
    expect(screen.queryByRole("button", { name: "来源 1" })).not.toBeInTheDocument();
    fireEvent.mouseOver(links[0]);
    expect(screen.queryByRole("group", { name: "引用预览" })).not.toBeInTheDocument();
    fireEvent.click(links[0]);
    expect(await screen.findByRole("dialog")).toHaveTextContent("海南美食指南.pdf");
    expect(screen.getByRole("dialog")).toHaveTextContent("文昌鸡是海南特色美食");
    expect(screen.getByRole("dialog").querySelector("a")).toBeNull();
  });

  it("routes citations to a non-modal side panel when the conversation owns the panel", () => {
    const onCitationSelect = vi.fn();
    const { rerender } = render(
      <MessageContent content={numberedAnswer} onCitationSelect={onCitationSelect} t={(key) => key} />,
    );
    fireEvent.click(screen.getAllByRole("button", { name: "查看引用：海南美食指南.pdf" })[0]);
    expect(onCitationSelect).toHaveBeenCalledOnce();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    const selection = onCitationSelect.mock.calls[0][0];
    rerender(
      <CitationSources
        activeID={selection.activeID}
        cited={selection.cited}
        onActiveChange={() => undefined}
        t={(key) => key}
        variant="panel"
      />,
    );
    const panel = screen.getByRole("complementary", { name: "来源" });
    expect(panel).toHaveTextContent("海南美食指南.pdf");
    expect(panel).toHaveTextContent("文昌鸡是海南特色美食");
    expect(panel).not.toHaveTextContent("来源 5");
  });

  it("shows only the clicked document in the side panel and closes on request", () => {
    const cited = renderMarkdownWithCitations(
      `甲 ([甲.pdf][1])，乙 ([乙.pdf][2])。\n\n[1]: "甲片段"\n[2]: "乙片段"`,
    ).cited;
    const onActiveChange = vi.fn();
    render(
      <CitationSources activeID="2" cited={cited} onActiveChange={onActiveChange} t={(key) => key} variant="panel" />,
    );
    const panel = screen.getByRole("complementary", { name: "来源" });
    expect(panel).toHaveTextContent("乙.pdf");
    expect(panel).toHaveTextContent("乙片段");
    expect(panel).not.toHaveTextContent("甲片段");
    fireEvent.click(screen.getByRole("button", { name: "关闭" }));
    expect(onActiveChange).toHaveBeenCalledWith(null);
  });

  it("restores, resizes, and saves the source panel width", () => {
    window.localStorage.setItem(CONVERSATION_CITATION_PANEL_WIDTH_STORAGE_KEY, "340");
    const cited = renderMarkdownWithCitations(numberedAnswer).cited;
    const { unmount } = render(
      <CitationSources activeID="5" cited={cited} onActiveChange={() => undefined} t={(key) => key} variant="panel" />,
    );
    const panel = screen.getByRole("complementary", { name: "来源" });
    const handle = screen.getByRole("separator", { name: "调整来源面板宽度" });
    expect(panel.style.getPropertyValue("--conversation-citation-panel-width")).toBe("340px");
    fireEvent.keyDown(handle, { key: "ArrowLeft" });
    expect(panel.style.getPropertyValue("--conversation-citation-panel-width")).toBe("364px");
    expect(window.localStorage.getItem(CONVERSATION_CITATION_PANEL_WIDTH_STORAGE_KEY)).toBe("364");

    Object.defineProperties(handle, {
      setPointerCapture: { value: vi.fn() },
      hasPointerCapture: { value: () => true },
      releasePointerCapture: { value: vi.fn() },
    });
    fireEvent.pointerDown(handle, { pointerId: 1, clientX: 500 });
    fireEvent.pointerMove(handle, { pointerId: 1, clientX: 460 });
    fireEvent.pointerUp(handle, { pointerId: 1, clientX: 460 });
    expect(panel.style.getPropertyValue("--conversation-citation-panel-width")).toBe("404px");
    expect(window.localStorage.getItem(CONVERSATION_CITATION_PANEL_WIDTH_STORAGE_KEY)).toBe("404");
    unmount();

    render(<CitationSources activeID="5" cited={cited} onActiveChange={() => undefined} variant="panel" />);
    expect(screen.getByRole("complementary", { name: "来源" })).toHaveStyle({
      "--conversation-citation-panel-width": "404px",
    });
    window.localStorage.removeItem(CONVERSATION_CITATION_PANEL_WIDTH_STORAGE_KEY);
  });
});
