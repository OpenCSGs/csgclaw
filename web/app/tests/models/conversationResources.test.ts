import {
  resourcesMatchAgent,
  addResources,
  canSendComposer,
  messageResources,
  resourceMessageInput,
  resourceMessageText,
  resourceSegment,
  type ResourceSnapshot,
} from "@/models/conversationResources";
import {
  areComposerSegmentsEqual,
  getComposerSlashQueryAtSelection,
  placeCaretAtEnd,
  replaceComposerSlashWithSegments,
  parseComposerSegments,
  renderComposerSegments,
  serializeComposerSegments,
} from "@/models/composer";

const skill: ResourceSnapshot = { source: "skill", id: "review", type: "skill", name: "review" };
const app: ResourceSnapshot = { source: "app", id: "installation-a", type: "connector", name: "GitLab" };

describe("会话资源选择", () => {
  it("相同资源去重，并保持正文和不同来源的身份", () => {
    const segments = addResources(
      [{ type: "text", text: "检查 MR" }],
      [skill, skill, app, { ...app, id: "installation-b" }],
    );
    const payload = resourceMessageInput(segments);
    expect(payload.content).toBe("检查 MR");
    expect(payload.resource_refs).toEqual([
      { source: "skill", id: "review" },
      { source: "app", id: "installation-a" },
      { source: "app", id: "installation-b" },
    ]);
    expect(resourceMessageInput([]).resource_refs).toEqual([]);
  });
  it("将历史格式与标签选择转换为相同的发送数据", () => {
    expect(resourceMessageInput([], '<slash-command name="use-skill" arg="review"></slash-command> 检查')).toEqual(
      resourceMessageInput([resourceSegment(skill), { type: "text", text: "检查" }]),
    );
    expect(resourceMessageInput([], '<slash-command name="new" arg="conversation"></slash-command>').content).toContain(
      'name="new"',
    );
  });
  it("空正文仅允许单个技能或附件", () => {
    expect(canSendComposer([resourceSegment(skill)], 0)).toBe(true);
    expect(canSendComposer([resourceSegment(app)], 0)).toBe(false);
    expect(canSendComposer([resourceSegment(app)], 1)).toBe(true);
    expect(canSendComposer([resourceSegment(skill), resourceSegment(app)], 0)).toBe(false);
    expect(canSendComposer([resourceSegment(skill), resourceSegment(skill)], 0)).toBe(true);
  });
  it("编辑器往返保持引用，正文序列化排除标签", () => {
    const root = document.createElement("div");
    const segments = addResources([{ type: "text", text: "正文" }], [{ ...app, name: '<script>"名称"</script>' }]);
    renderComposerSegments(root, segments);
    expect(root.querySelector("script")).toBeNull();
    const parsed = parseComposerSegments(root);
    expect(areComposerSegmentsEqual(parsed, segments)).toBe(true);
    expect(serializeComposerSegments(parsed)).toBe("正文");
    root.querySelector("[data-composer-resource]")?.remove();
    expect(resourceMessageInput(parseComposerSegments(root)).resource_refs).toEqual([]);
  });
  it("历史展示读取发送时快照，复制文本包含资源名称", () => {
    const metadata = { csgclaw: { resource_selection: { version: 1, agent_id: "agent", refs: [skill, app] } } };
    expect(messageResources(metadata)).toEqual([skill, app]);
    expect(resourceMessageText("正文", metadata)).toBe("[review] [GitLab] 正文");
    expect(messageResources({ csgclaw: { resource_selection: { version: 2, refs: [skill] } } })).toEqual([]);
  });
  it("草稿保留 Agent 身份，切换目标后拒绝旧选择", () => {
    const segments = addResources([], [skill], "agent-a");
    const root = document.createElement("div");
    renderComposerSegments(root, segments);
    expect(resourcesMatchAgent(parseComposerSegments(root), "agent-a")).toBe(true);
    expect(resourcesMatchAgent(parseComposerSegments(root), "agent-b")).toBe(false);
    expect(canSendComposer([...segments, { type: "slash", text: "/new" }], 0)).toBe(false);
  });
});

it("资源标签之后可以选择 slash 技能，已有正文仍然保留查询限制", () => {
  const root = document.createElement("div");
  root.contentEditable = "true";
  document.body.append(root);
  try {
    renderComposerSegments(root, [resourceSegment(app, "agent"), { type: "text", text: " /rev" }]);
    placeCaretAtEnd(root);
    expect(getComposerSlashQueryAtSelection(root)).toBe("rev");
    expect(replaceComposerSlashWithSegments(root, [resourceSegment(skill, "agent"), { type: "text", text: " " }])).toBe(
      true,
    );
    expect(resourceMessageInput(parseComposerSegments(root)).resource_refs).toEqual([
      { source: "app", id: app.id },
      { source: "skill", id: skill.id },
    ]);
    renderComposerSegments(root, [resourceSegment(app, "agent"), { type: "text", text: "已有正文 /rev" }]);
    placeCaretAtEnd(root);
    expect(getComposerSlashQueryAtSelection(root)).toBeNull();
  } finally {
    root.remove();
    window.getSelection()?.removeAllRanges();
  }
});
