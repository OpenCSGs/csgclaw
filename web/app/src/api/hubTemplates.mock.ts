import type { HubTemplate } from "@/models/hubWorkspace";

const names = [
  "代码审查助手",
  "前端开发助手",
  "后端开发助手",
  "自动化测试助手",
  "技术文档助手",
  "数据分析助手",
  "产品需求分析助手",
  "用户体验设计助手",
  "项目管理助手",
  "知识库问答助手",
  "客户服务助手",
  "市场调研助手",
  "内容创作助手",
  "多语言翻译助手",
  "会议纪要助手",
  "数据库优化助手",
  "安全审计助手",
  "持续集成与部署助手",
  "日志分析助手",
  "性能诊断助手",
  "API 集成助手",
  "开源项目维护助手",
  "发布说明生成助手",
  "工作周报助手",
  "竞品分析助手",
  "跨团队需求梳理与任务拆解助手",
  "复杂业务流程自动化助手",
  "企业知识整理与检索助手",
  "多仓库代码迁移与兼容性检查助手",
  "端到端产品交付协作助手",
];

const descriptions = [
  "快速完成日常任务。",
  "结合项目上下文梳理问题，提供清晰的分析结果与可执行建议。",
  "面向复杂业务场景，协助收集资料、分析需求、拆解任务并验证结果，让团队能够持续跟进每个环节的进展与交付质量。",
];

export const mockHubTemplates: HubTemplate[] = names.map((name, index) => ({
  id: `mock-template-${String(index + 1).padStart(2, "0")}`,
  name,
  description: descriptions[index % descriptions.length],
  role: "worker",
  runtime_kind: "codex",
  source: [
    { name: "official", kind: "remote" },
    { name: "local", kind: "local" },
    { name: "builtin", kind: "builtin" },
  ][index % 3],
}));
