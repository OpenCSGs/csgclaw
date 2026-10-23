---
name: paddleocr-md
description: 使用 PaddleOCR 异步 API 将 PDF、图片（扫描件/截图/拍照等）OCR 识别并转为 Markdown 文档。支持多页 PDF，自动输出结构化的 Markdown + 嵌入图片。当用户需要将不可复制的 PDF、扫描件、图片合同、发票、报表等文档转化为可编辑的 Markdown 格式时使用。
---

# PaddleOCR → Markdown

将 PDF（扫描件 / 数字 PDF）、图片等文件提交至 PaddleOCR API 进行 OCR 识别，输出为结构化的 Markdown 文档。

## 前置条件

- Python 3.8+
- 已安装 `requests` 库
- 已配置 PaddleOCR API Token（详见下节，**本 Skill 不内置 Token**）

## Token 准备（执行前必做）

本 Skill 不内置任何 Token。脚本按以下顺序解析 Token，全部缺失时直接退出（退出码 2）并打印获取指引：

1. 命令行参数 `--token`
2. 环境变量 `PADDLE_OCR_TOKEN`
3. Token 文件 `~/.config/paddleocr/api_key`（可用 `PADDLE_OCR_KEY_FILE` 改路径）

### 智能体执行流程

调用任何脚本之前，必须按顺序完成以下步骤，不要跳过检查、也不要假设 Token 已存在：

1. **一条命令查清现状**：`python scripts/paddleocr_key.py check`。它会打印「已找到 Token（掩码）/ 来源」，找到就直接执行 OCR，不要重复向用户索要。
2. **缺失时向用户索取**：提示用户打开 <https://aistudio.baidu.com/account/accessToken> 获取 Access Token（百度 AI Studio 的 PaddleOCR 访问令牌），把 Token 发给你。说明 Token 仅在本机用于调用 OCR 接口，不会写入仓库文件。
3. **拿到后写入 Token 文件，不要改 shell 配置**：`python scripts/paddleocr_key.py set --token <Token>`。脚本会写入 `~/.config/paddleocr/api_key` 并设为 600 权限（仅当前用户可读），终端进程与 GUI 应用都能读到。
4. **复核后执行**：再跑一次 `paddleocr_key.py check` 确认来源正确，然后正常调用 OCR 脚本。

需要清理时用 `paddleocr_key.py delete`。

### 为什么不写 shell 配置文件

shell 配置在不同系统、不同启动方式下差异很大，对 GUI 里的 Agent 往往无效：

| 系统 | 配置文件 | 对 GUI 应用 |
|---|---|---|
| macOS | zsh 的 `~/.zshrc`（交互非登录）、`~/.zprofile`（登录）、`~/.zshenv`（所有 zsh） | **读不到**，GUI 应用由 launchd 启动，不执行 shell rc |
| Linux | `~/.bashrc`（交互非登录）、`~/.profile`（登录） | 通常读不到，桌面会话有独立环境 |
| Windows | 无 shell rc 概念，用 `setx` 写注册表用户环境变量 | 能读到，但对已打开的终端无效 |

同理 `export VAR=...` 只对当前 shell 会话有效。所以本 Skill 首选「Token 文件」这条与环境无关的路径；环境变量作为备选，仅在明确知道运行进程会读到它时使用。

**安全要求**：Token 只能出现在 Token 文件、环境变量或当次 `--token` 参数中；不得写入本 SKILL.md、脚本、仓库其他文件、提交记录或日志。向用户复述时始终掩码显示（如 `1234****cdef`），不要回显完整 Token。若脚本提示「权限过宽」，让用户执行 `chmod 600 ~/.config/paddleocr/api_key`。

**Token 失效处理**：若脚本返回 401/403 或鉴权失败，说明 Token 过期或被重置，提示用户重新到 <https://aistudio.baidu.com/account/accessToken> 获取，并按上述步骤 3 覆盖旧值。

### `scripts/paddleocr_key.py` — Token 管理

```bash
python scripts/paddleocr_key.py set --token <Token>   # 写入 ~/.config/paddleocr/api_key（600）
python scripts/paddleocr_key.py check                 # 查看当前 Token 来源（掩码）
python scripts/paddleocr_key.py delete                # 删除已保存的 Token
```

## 脚本

### `scripts/paddleocr_to_md.py` — 单文件处理

一键 OCR 识别并转为 Markdown 文档。

**支持的输入：**
- **本地文件**：PDF、PNG、JPG、JPEG、BMP、TIFF 等（单文件 ≤ 50 MB）
- **在线文件 URL**：同上（单文件 ≤ 200 MB，多页 PDF 最多 1000 页）

**支持的模型：**
- `PaddleOCR-VL-1.6`（默认，推荐）
- `PaddleOCR-VL-1.4`
- `PaddleOCR-VL-2`
- `PaddleOCR-VL-1.1`

#### 用法

```bash
# 本地 PDF/图片
python scripts/paddleocr_to_md.py ./扫描件.pdf

# 在线文件 URL
python scripts/paddleocr_to_md.py https://example.com/doc.pdf

# 指定模型
python scripts/paddleocr_to_md.py ./发票.png --model PaddleOCR-VL-2

# 禁用展开矫正（适合非弯曲文档以加快处理）
python scripts/paddleocr_to_md.py ./doc.pdf --no-unwarping

# 不下载图片
python scripts/paddleocr_to_md.py ./扫描件.pdf --skip-images --skip-layout-images

# 手动指定输出目录
python scripts/paddleocr_to_md.py ./合同.jpg --output-dir ./my_output
```

#### 可选参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--token TOKEN` | API Token（默认读环境变量 `PADDLE_OCR_TOKEN` 或 `~/.config/paddleocr/api_key`；缺失时报错并提示获取地址） | 无，必填 |
| `--model MODEL` | OCR 模型名称 | `PaddleOCR-VL-1.6` |
| `--output-dir DIR` | 输出目录（默认自动生成: `{文件名}_{时间戳}`） | 自动生成 |
| `--poll-interval N` | 轮询间隔（秒） | `5` |
| `--timeout N` | 最大等待时间（秒） | `600` |
| `--no-orient-classify` | 禁用文档方向分类 | `False` |
| `--no-unwarping` | 禁用文档展开矫正 | `False` |
| `--no-chart-reco` | 禁用图表识别 | `False` |
| `--skip-images` | 不下载 Markdown 中引用的图片 | `False` |
| `--skip-layout-images` | 不下载版面检测结果图片 | `False` |

#### 输出说明

输出目录自动命名为 `{原文件名}_{时间戳}`（如 `执委会会议记录_20260729_095712`），生成以下文件：

- `{原文件名}.md` — 所有页面合并为一个 Markdown 文件
- `imgs/` — Markdown 中通过 `![...](...)` 或 `<img src="...">` 引用的图片（如有）
- `layout_det_res_N.jpg` — 版面检测结果图片（如有）

**图片处理原则**：只下载 Markdown 文本中实际引用的图片（支持 `![...](...)` 和 `<img src="...">` 两种语法）。API 返回的页眉 logo 等未在文本中引用的图片会被自动忽略。

---

### `scripts/paddleocr_batch.py` — 批量处理

批量处理大量 PDF/图片文件，支持并发提交、实时进度、中断恢复和汇总报告。适合几十、几百甚至上千个文档的批量 OCR 场景。

#### 用法

```bash
# glob 模式匹配
python scripts/paddleocr_batch.py ./合同/*.pdf

# 递归扫描目录
python scripts/paddleocr_batch.py ./全部文档/

# 多个输入
python scripts/paddleocr_batch.py ./pdfs/*.pdf ./扫描件/*.png

# 控制并发数
python scripts/paddleocr_batch.py ./文件/ --max-concurrent 5

# 从上次中断处继续
python scripts/paddleocr_batch.py ./文件/ --resume

# 指定输出目录
python scripts/paddleocr_batch.py ./文件/*.pdf --output-dir ./ocr_batch
```

#### 可选参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `INPUT ...` | 文件路径、glob 模式或目录（支持多个） | 必填 |
| `--token TOKEN` | API Token（默认读环境变量 `PADDLE_OCR_TOKEN` 或 `~/.config/paddleocr/api_key`；缺失时报错并提示获取地址） | 无，必填 |
| `--model MODEL` | OCR 模型名称 | `PaddleOCR-VL-1.6` |
| `--output-dir DIR` | 批量输出根目录 | `./paddleocr_batch_output` |
| `--max-concurrent N` | 最大并发任务数（推荐 3-5） | `3` |
| `--timeout N` | 单个任务最大等待秒数 | `600` |
| `--no-orient-classify` | 禁用文档方向分类 | `False` |
| `--no-unwarping` | 禁用文档展开矫正 | `False` |
| `--no-chart-reco` | 禁用图表识别 | `False` |
| `--skip-images` | 不下载图片 | `False` |
| `--skip-layout-images` | 不下载版面检测图片 | `False` |
| `--resume` | 从上次中断处继续 | `False` |
| `--extensions` | 处理的文件扩展名（逗号分隔） | `.pdf,.png,.jpg,.jpeg,.bmp,.tiff,.tif` |

#### 批量输出结构

```
输出根目录/
├── 文件1_名称/          # 每个文件一个独立子目录
│   └── 文件1_名称.md
├── 文件2_名称/
│   └── 文件2_名称.md (含 imgs/ 如有引用图片)
├── ...
├── _batch_state.json    # 运行状态（支持中断恢复）
└── _batch_report.json   # 最终汇总报告
```

#### 关键特性

| 特性 | 说明 |
|---|---|
| **并发控制** | `--max-concurrent` 控制同时提交的任务数，避免压垮 API |
| **中断恢复** | `--resume` 从上次中断处继续，已完成的文件自动跳过 |
| **进度显示** | 实时显示进度条、成功/失败计数、预计剩余时间 |
| **汇总报告** | 运行结束后生成 `_batch_report.json`，含每个文件的页数、路径、耗时 |
| **自动过滤** | 自动跳过超过 50MB 的文件，只处理支持的扩展名 |

#### 批量处理建议

- **小批量（< 50 文件）**：`--max-concurrent 3`（默认即可）
- **中批量（50-500 文件）**：`--max-concurrent 5`
- **大批量（> 500 文件）**：`--max-concurrent 5`，建议分批运行 + `--resume`
- 每个文件的平均处理时间取决于页数和服务排队情况（通常几秒到几分钟）
- 如果网络中断或脚本异常退出，加上 `--resume` 重新运行即可从断点继续

## 流程说明

1. **单文件**：提交 → 轮询 → 下载合并为一个 `.md` 文件
2. **批量**：扫描文件 → 并发提交 → 并发轮询 → 每个文件独立输出 → 生成汇总报告
