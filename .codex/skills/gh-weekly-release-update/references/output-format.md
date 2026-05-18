# Weekly Update Output Format

Use this structure unless the user asks for another format. Save the final result as a Markdown file under `docs/weekly-releases`.

Recommended filenames:

- Range release: `v0.3.1-v0.3.2.md`
- Single release: `v0.3.2.md`

## Chinese

```md
## 本周发布更新

一句话总结本周发布重点，并自然带出本次覆盖的版本范围。

### 更新内容

- 将每个重要功能点或重要修复点各自单独列成一条，不要把多个不同点揉成一个大段落。
- 同一主题下如果是两个独立能力，也要拆开写，例如两个不同 runtime、两个不同入口或两个不同工作流改进。
- 只保留独立且值得注意的更新点，不要把零碎的小改动拆得过细。
- 每条尽量控制为一句话，先说变化，再说影响。
- 优先写体验提升、稳定性修复、流程优化、能力扩展。
- 如果有技术细节，先说影响，再补一句必要背景。
- 即使几个点属于同一主题，也优先拆开写，只在完全重复时合并。

### 需要注意

- 只在确实存在普通用户需要知道的破坏性变更、配置调整、升级建议或已知限制时保留这一节。
- 如果没有明确风险或动作项，直接省略这一节。

### 涉及版本

- v0.3.1: ...
- v0.3.2: ...
```

更新内容示例：

```md
## 更新内容

- 新增支持 Docker Sandbox，可以基于 Docker 运行 CSGClaw Agent，比 BoxLite 更稳定。
- 正式支持 Windows 平台原生安装，无需 WSL2，但仍需要安装 Docker。
- Web UI 支持在线升级进度和失败状态展示，并优化了按钮、输入框、Popover 等交互体验。
- Manager 内置 Feishu 技能，可通过 Manager 聊天引导用户将 CSGClaw 接入飞书渠道。
- Manager 内置 Skill-Creator 技能，支持创建新技能。
- Manager 内置 Skill-Installer 技能，支持从 Git 仓库自动安装技能。
- 优化 CLIProxy Auth 自动刷新机制，减少因登录失效导致的异常。
- 支持保存和恢复 Agent 模型配置，Worker 创建时可自动继承 Agent Profile 配置。
- 优化 IM Event Stream 保活机制，提升消息流连接稳定性。
```

这个示例体现的原则：

- 每个独立能力或改进点单独成条。
- 每条尽量一句话，说清变化和价值。
- 可以保留少量必要的技术名词，但不要展开成长解释。
- 同一主题下如果是不同能力，仍然拆开写。

## English

```md
## Weekly Release Update

One plain-English sentence that explains the overall theme and naturally mentions the covered release range.

### What's Changed

- Write 3-7 user-facing highlights.
- Split sibling capabilities into separate bullets when users would reasonably see them as different updates.
- Keep the list selective. Include distinct, notable updates instead of every small change.
- Keep each bullet as short as possible; prefer one sentence.
- Give each important feature or fix its own bullet when users would benefit from seeing it separately.
- Prefer outcomes over subsystem names.
- If a point is technical, explain why it matters to users or operators.

### Notes

- Keep this section only when there is a clear user-facing breaking change, upgrade action, configuration update, or known limitation.
- Omit it when normal users do not need to do anything.

### Covered releases

- v0.3.1: ...
- v0.3.2: ...
```

## Style Rules

- Keep both languages aligned in meaning, but do not translate word-for-word.
- Avoid raw commit-style phrasing such as `feat:` or `fix:`.
- Replace internal jargon with user language when possible.
- Use short paragraphs and compact bullets.
- Prefer concise, flowing phrasing over exhaustive explanation.
- Prefer one update per bullet over thematic bundling.
- Cut filler and repeated framing across bullets.
- Prefer specific bullets over broad umbrella summaries.
- If no upgrade action is needed, say so briefly instead of creating filler.
