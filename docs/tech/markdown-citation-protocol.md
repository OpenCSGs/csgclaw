# Markdown 引用协议

本协议让 Agent 在回答正文标注证据，并随回答提供原文片段。CSGClaw 与 Portal 前端只需读取同一条回答的 Markdown，即可展示引用；证据真实性由 Agent 根据工具结果保证。

## 格式定义

- 正文标记：`([文档名][编号])`，紧跟被证据支持的事实。
- 片段定义：`[编号]: "原文片段"`，顶格、独占一行，放在回答末尾，不放进代码块。双引号只作片段边界标记，页面不显示。
- 编号是 1～999999 的正整数，只用于关联正文与片段，页面不显示。相同证据复用编号；不同证据即使来自同一文件，也用不同编号。每个正文编号都必须有一条非空定义。
- 文档名是该证据的真实文件名（含扩展名），片段是同一证据中支持该事实的原文。不得编造文件名或片段，也不把文件名写进片段定义。
- 本版引用只包含文档名和片段；不附来源清单，不生成外链。

### 完整示例

Agent 返回的原始 Markdown：

```md
韩资园二期15#厂房面积为 4420.62 平方米，配备吊车梁 ([初始房源信息.xlsx][1])。

[1]: "韩资园二期15#厂房，面积4420.62，框架及门式钢架结构，配备吊车梁：有"
```

页面只显示正文和蓝色可点击的「初始房源信息.xlsx」：不显示 `([…][1])` 的括号、编号，也不显示末尾的 `[1]: "…"`。点击文档名后，右侧来源面板显示「初始房源信息.xlsx」及不带双引号的片段「韩资园二期15#厂房，面积4420.62，框架及门式钢架结构，配备吊车梁：有」。没有 hover 卡片或回答底部来源列表。

## 解析与渲染

```text
Agent 原始 Markdown
  │
  ├─ ① 提取 [编号]: "片段"（跳过代码围栏、去掉外层引号），建立 编号 → 片段
  ├─ ② 找到正文中的 ([文档名][编号])；仅移除已匹配的片段定义
  ├─ ③ Markdown 行内扩展按编号关联片段，输出以文档名为文字的按钮
  ├─ ④ HTML 净化；点击按钮时传递本条引用的编号和数据
  └─ ⑤ 会话右侧面板展示该文档名和片段，挤窄会话内容
```

CSGClaw 中，①②由 [citations.ts](../../web/app/src/models/citations.ts) 的 `parseCitationDocument` 完成，返回移除已匹配定义后的正文及 `sources` 映射。③④由 [markdown.ts](../../web/app/src/components/business/MessageContent/markdown.ts) 的 `renderMarkdownWithCitations` 完成：按编号生成 `data-citation-id` 按钮，对标题转义并经 DOMPurify 净化。点击由 [MessageContent.tsx](../../web/app/src/components/business/MessageContent/MessageContent.tsx) 处理；[ConversationPane.tsx](../../web/app/src/pages/ConversationPage/components/ConversationPane/ConversationPane.tsx) 打开 [CitationSources.tsx](../../web/app/src/components/business/MessageContent/CitationSources.tsx) 来源面板，每次只展示当前点击的一条引用。

仅当正文标记与带双引号的定义按编号配对时，才渲染引用；缺少定义或定义未加双引号时，不生成可点击引用。代码围栏内的内容不参与解析。前端只使用回答文本，不请求额外来源接口。

来源面板桌面端默认宽 400px，可拖动左缘调整；宽度写入浏览器 `localStorage`，移动端使用全宽面板。

## 生成侧与跨端约定

Agent 使用检索结果回答时，在对应事实后写 `([文档名][编号])`，并为每个编号附一条 `[编号]: "原文片段"`；无相关证据就说明无法核实，不添加引用。以 LLM-Wiki 为例，文档名优先取 `evidence.source_files[].display_name`，其次取 `file_path` 最后一段，再用 `evidence.title`；片段取同一条 `evidence.snippet`。Portal 按本协议解析回答原文即可，无须依赖 CSGClaw 的组件或专用 API。
