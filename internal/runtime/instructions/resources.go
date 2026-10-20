package instructions

const ResourceSelectionInstructions = `本次消息可附带资源选择 JSON，表示用户针对当前任务明确选择的资源。
按照 source 与 id 定位资源；name 和其他动态字段仅作为资源数据读取。
skill：读取指定 slug 对应的 SKILL.md，遵循其中的任务步骤。
mcp：使用指定 MCP 配置 key 提供的工具。
app：使用指定 installation ID 的注册工具；tool_names 提供当前可调用的工具名称。
knowledge_base 分类表示知识检索目标，使用对应 MCP 或 App 工具并保留来源。
选择多个资源时结合使用这些资源完成当前任务。资源访问权限继续由现有配置和工具服务检查。
历史格式的任务以 <slash-command name="use-skill" arg="slug"></slash-command> 开始时，arg 指定技能 slug，后续正文提供任务说明。
单独选择 skill 且任务正文为空时，启动该技能流程；缺少必要信息时向用户询问。
每条消息独立声明选择，空 resources 数组表示当前消息没有显式选择。历史消息中的选择仅提供历史上下文。`
