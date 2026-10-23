---
name: word-to-md
description: 将 Word 文档（.doc / .docx）转换为 Markdown 并提取内嵌图片。自动处理 .doc（通过 Word COM 转为 .docx）和 .docx（通过 mammoth 直接转换）。当用户需要将 Word 文档转为可编辑的 Markdown 格式、从 Word 文档中提取图片、或将 Word 内容迁移到 Markdown 编辑器时使用。
---

# Word → Markdown

将 Word 文档（.doc / .docx）转换为结构化的 Markdown 文档，并自动提取内嵌图片。

## 前置条件

- Python 3.8+
- 已安装依赖：`pip install mammoth olefile pywin32 pillow python-docx`

## 脚本

### `scripts/doc_to_md.py` — 单文件转换

一键将 Word 文档转为 Markdown 并提取图片。

**支持的输入：**
- `.docx` — 直接使用 mammoth 转换，保留格式/表格/图片
- `.doc` — 通过 win32com（Microsoft Word）转为 .docx 后由 mammoth 处理；若 Word 不可用则回退到 olefile 提取纯文本

#### 用法

```bash
# .docx 文档
python scripts/doc_to_md.py 文档.docx

# .doc 文档（需要安装 Word）
python scripts/doc_to_md.py 旧版文档.doc

# 指定输出目录
python scripts/doc_to_md.py 文档.docx --output-dir ./my_output
```

#### 可选参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `INPUT` | 输入的 Word 文件路径（.doc / .docx） | 必填 |
| `--output-dir, -o DIR` | Markdown 输出父目录（自动创建子目录） | 当前目录 |

#### 输出说明

输出目录自动命名为 `{原文件名}_{时间戳}`（如 `项目方案_20260729_095712`），生成以下文件：

- `{原文件名}.md` — 转换后的 Markdown 文件
- `imgs/` — 文档中内嵌的图片（通过 `![](imgs/image_N.ext)` 引用），**仅当文档包含图片时生成**

**图片处理原则**：对文档中的每一张内嵌图片自动按 `image_1.png`、`image_2.jpg` 等规则命名，存入 `imgs/` 目录。Markdown 中使用相对路径 `![](imgs/image_1.png)` 引用。如果文档不包含图片，则不生成 `imgs/` 目录，Markdown 中也不包含图片引用。

#### 典型输出结构

```
项目方案_20260729_095712/
├── 项目方案.md
└── imgs/               # 仅当有图片时出现
    ├── image_1.png
    └── image_2.jpg
```

## 流程说明

1. 识别输入文件类型（.doc / .docx）
2. 如果是 .doc，通过 Word COM 转换为临时 .docx
3. 使用 mammoth 将 .docx 转换为 Markdown，同时自定义图片处理器将图片保存到 `imgs/` 目录
4. 若文档无图片，不生成 `imgs/` 目录
5. 输出最终目录结构
