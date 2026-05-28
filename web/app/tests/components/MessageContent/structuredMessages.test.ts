import {
  ACTION_REBUILD_MANAGER,
  CSGCLAW_ACTION_CARD_TYPE,
  CSGCLAW_NOTIFY_CARD_TYPE,
} from "@/shared/constants/messages";
import {
  extractTopLevelJSONObject,
  parseStructuredMessage,
  tryParseJSON,
} from "@/components/business/MessageContent/structuredMessages";

describe("structured message helpers", () => {
  it("parses fenced action-card JSON and keeps only supported actions", () => {
    const parsed = parseStructuredMessage(`\`\`\`json
{
  "type": "${CSGCLAW_ACTION_CARD_TYPE}",
  "title": "Manager unavailable",
  "status": "Action required",
  "actions": [
    { "id": "ignored", "label": "Ignored" },
    { "id": "${ACTION_REBUILD_MANAGER}", "label": "Rebuild now", "style": "danger" }
  ]
}
\`\`\``);

    expect(parsed).toMatchObject({
      actions: [
        {
          id: ACTION_REBUILD_MANAGER,
          label: "Rebuild now",
          style: "danger",
        },
      ],
      badge: "Action required",
      kind: "action_card",
      title: "Manager unavailable",
    });
  });

  it("does not promote embedded generic JSON into a structured payload", () => {
    const extracted = extractTopLevelJSONObject('Before {"tool":"read_file","arguments":{"path":"README.md"}} after');

    expect(extracted).toBe('{"tool":"read_file","arguments":{"path":"README.md"}}');
    expect(parseStructuredMessage(`Assistant result: ${extracted}`)).toBeNull();
  });

  it("extracts action-card JSON from surrounding assistant text", () => {
    const parsed = parseStructuredMessage(`Manager setup complete:
{
  "type": "${CSGCLAW_ACTION_CARD_TYPE}",
  "title": "Manager needs rebuild",
  "actions": [
    { "id": "${ACTION_REBUILD_MANAGER}", "label": "Rebuild manager" }
  ]
}`);

    expect(parsed).toMatchObject({
      actions: [
        {
          id: ACTION_REBUILD_MANAGER,
          label: "Rebuild manager",
        },
      ],
      kind: "action_card",
      title: "Manager needs rebuild",
    });
  });

  it("leaves legacy PicoClaw tool feedback as markdown", () => {
    expect(
      parseStructuredMessage(`🔧 \`read_file\`
\`\`\`json
{"path":"README.md"}
\`\`\``),
    ).toBeNull();
  });

  it("parses notifier notify-card payloads with links and meta rows", () => {
    const parsed = parseStructuredMessage(
      JSON.stringify({
        type: CSGCLAW_NOTIFY_CARD_TYPE,
        title: "GitLab · Merge request",
        subtitle: "acme/app",
        badge: "open",
        summary: "Ready for review",
        link: "https://gitlab.example/acme/app/-/merge_requests/1",
        meta: [
          { label: "标题", value: "Fix bug" },
          { label: "empty", value: "" },
        ],
        raw: '{"hello":"world"}',
      }),
    );

    expect(parsed).toMatchObject({
      badge: "open",
      link: "https://gitlab.example/acme/app/-/merge_requests/1",
      meta: [
        { label: "标题", value: "Fix bug" },
        { label: "empty", value: "" },
      ],
      payload: '{"hello":"world"}',
      payloadSummary: "查看原始 JSON",
      subtitle: "acme/app",
      summary: "Ready for review",
      title: "GitLab · Merge request",
    });
  });

  it("returns null for invalid or non-object structured content", () => {
    expect(tryParseJSON("not json")).toBeNull();
    expect(tryParseJSON("{bad json")).toBeNull();
    expect(parseStructuredMessage("plain markdown")).toBeNull();
  });

  it("collapses large code blocks into structured code payloads", () => {
    const code = Array.from({ length: 20 }, (_, index) => `console.log(${index});`).join("\n");
    const parsed = parseStructuredMessage(`\`\`\`ts
${code}
\`\`\``);

    expect(parsed).toMatchObject({
      badge: "Code",
      code,
      codeSummary: "展开代码 · 20 行",
      subtitle: "TS",
      title: "Long code block",
    });
  });

  it("summarizes code-looking fields in structured payloads", () => {
    const code = `function run() {\n${"  console.log('hello');\n".repeat(12)}}`;
    const parsed = parseStructuredMessage(
      JSON.stringify({
        action: "write_file",
        arguments: { path: "src/App.tsx", replace: true },
        code,
        status: "ok",
      }),
    );

    expect(parsed).toMatchObject({
      badge: "ok",
      code,
      subtitle: "",
      title: "write_file",
    });
    expect(parsed?.summary).toContain("参数: path, replace");
    expect(parsed?.summary).toContain("代码:");
  });
});
