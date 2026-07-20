# Subskill：API / 部署侧测试（无 MR）

用于已部署服务测试；配置来自项目根 `AgenticHub-test.yaml`（或用户选择后的 **AG** 生成脚本）。
**强制**：无 MR 上下文下的「全量/全部/完整测试」走本子 skill；不运行 `detect.py`。

其中 `SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"`。

**认证**：通过 Connector lease 执行业务；遇到 401/403 时停止重试，并按父 skill「GitLab 认证」提示用户检查 Connector。

## 基本规则

- 测试目标是已部署服务，不是本地 MR 双分支回归。
- 不运行 `detect.py`，不做技术栈探测流程。
- 不安装项目依赖，除非 `AgenticHub-test.yaml` 的 `setup` 或 **AG3** 中执行生成测试所需的最小依赖明确要求。
- 命令执行前先匹配用户意图对应 `commands` key（`smoke` / `contract` / `scenario` / `full` 或自定义 key 的语义匹配）。

## AgenticHub-test.yaml（参考）

| 字段 | 必填 | 说明 |
|------|------|------|
| `commands` | 是 | key → 一行 shell；key 可自定义，由意图语义匹配 |
| `setup` | 否 | 跑 `commands` 前的准备命令 |
| `report` | 否 | 结果文件路径（JUnit XML / JSON），便于 A6 解析 |

示例（节选）：

```yaml
setup: "pip install -r tests/requirements.txt"
commands:
  smoke: "pytest tests/ -m smoke -v"
  contract: "pytest tests/contract -v"
  full: "pytest tests/ -v"
```

---

## 有配置文件时：A1 → A7

### A1：克隆仓库（获取配置与脚本）

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab" && python3 scripts/setup_dev.py \
    "{project_path}" --branch "api-test-run"
```

若本会话已在开发/测试流中克隆过同一项目，可复用已有 `project_name` / 本地目录，不必重复克隆。

### A2：读取配置

```bash
cat ./gitlab_projects/{project_name}/AgenticHub-test.yaml
```

存在则解析 `setup`、`commands`、`report`，进入 A3。
不存在则见下文「配置缺失处理」。

### A3：匹配用户意图到 `commands` key

- 「冒烟 / P0」→ 优先 key 含 `smoke`
- 「契约 / 功能」→ 优先 `contract`
- 「场景 / 集成」→ 优先 `scenario`
- 「全量 / 全部 / 完整」→ 优先 `full`；无 `full` 则**依次执行**所有已定义 `commands`
- 自定义 key（如 `regression`）用语义理解匹配

**回退**：请求的 key 不存在时，先回退 `full`；无 `full` 再顺序执行全部 `commands`。

### A4：执行 `setup`（若有）

```bash
cd ./gitlab_projects/{project_name} && {setup_command}
```

### A5：执行匹配到的测试命令

```bash
cd ./gitlab_projects/{project_name} && {matched_command}
```

### A6：收集并解析结果

对 A5 中**每条**执行的命令构造一条 JSON 记录，至少包含：

- `command_key`, `command`, `status`, `exit_code`
- `stdout`, `stderr`（可截断但失败时须保留关键片段）
- `duration_seconds`（可选）
- `counts`：`passed` / `failed` / `skipped` / `errors` / `total`
- `failed_tests`：失败用例名与错误摘要

若 yaml 配置了 `report`，优先解析该路径；否则从 stdout 按 pytest / jest / go test 等惯例提取计数。**解析失败不得假定为通过**；保留原始输出。

### A7：生成报告

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-api-test" && python3 scripts/api_report.py \
    --project-info '{"project_name":"{project_name}","project_path":"{project_path}","project_id":{project_id}}' \
    --test-config '{yaml 解析 JSON}' \
    --test-results '{A6 JSON 数组}'
```

### A8：展示报告

将 A7 返回的 Markdown 报告展示给用户；`report_path` 可归档。全通过附简短摘要；有失败则指向失败用例段落。

---

## 配置缺失处理

若 A2 发现**没有** `AgenticHub-test.yaml`，必须给用户二选一（不得直接生成）：

1. **用户自行创建**后重试（推荐）——可将上文 YAML 示例发给用户。
2. **进入自动生成流程 AG**（见下）。

用户未选择前，不得执行 AG。

---

## 自动生成测试脚本（AG）

仅当用户明确选择「由我自动生成 / 选项 2」时进入。生成物**只在沙盒内临时存在**，不提交仓库；**不**以此绕过「无 MR 不测本地 MR 流」的父级规则：AG 仍针对**已部署服务**做 HTTP 类检查，与 A 流目标一致。

### AG1：分析项目

1. 用根目录文件判断技术栈：`pyproject.toml` / `requirements.txt` → Python；`package.json` → Node；`go.mod` → Go；`pom.xml` / `build.gradle` → Java。
2. 查找 API 入口（优先 OpenAPI/Swagger：`openapi.yaml`、`swagger.json`、`docs/`；否则 FastAPI/Express/Gin 等路由源码；再 README）。
3. **base_url**：从项目配置、环境变量或**直接询问用户**取得已部署服务根地址（禁止臆造）。

### AG2：生成测试脚本

- 目录：`./gitlab_projects/{project_name}/.csgbot-generated-tests/`
- 框架：Python 用 pytest；Node 用 jest；其他可用 shell + `curl`。
- 内容：**smoke**（端点存活 HTTP 200）；**contract**（有 OpenAPI 时做结构与参数校验，无 spec 可跳过 contract）。

生成前向用户确认范围（端点数、是否含 contract），用户拒绝则停止。

### AG3：执行并接入报告

1. 安装运行生成测试所需的**最小**依赖（如 `pip install httpx pytest`）。
2. 在 `project_name` 目录下执行生成测试命令，按 **A6** 同样结构收集每条结果 JSON。
3. 将 `test-config` 设为描述本次 AG 的 JSON（含 `source: "auto-generated"`、命令列表等），`test-results` 为 A6 数组，执行 **A7** `api_report.py`，再 **A8** 展示。

---

## 错误恢复（摘要）

- `setup_dev` / 测试命令失败：根据终端与 `exit_code` 说明原因；可修正配置或 base_url 后重试。
- `api_report.py` 失败：保留 A6 原始 JSON 与 stdout/stderr，重跑 A7。
