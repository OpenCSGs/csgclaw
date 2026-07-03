# 构建与发布

本文说明仓库中的构建、测试与发布命令。

## 默认构建

```bash
make
# 等价于：make build
```

默认构建会：

1. 将 Web UI 构建到 `web/static-dist/`。
2. 构建 `bin/csgclaw` 和宿主平台的 `bin/csgclaw-cli`。
3. 按当前 CPU 架构构建静态 Linux `csgclaw-cli`，安装到 `~/.csgclaw/sandbox-tools/csgclaw-cli`。

`make build-all` 保留为 `make build` 的别名。CSGClaw 不再在本地构建派生的 PicoClaw/OpenClaw 镜像。

常用 target：

| Target | 说明 |
|---|---|
| `make build-server-bin` | 构建 `bin/csgclaw` 和宿主平台的 `bin/csgclaw-cli` |
| `make install-sandbox-cli` | 将 Linux `csgclaw-cli` 构建到 `~/.csgclaw/sandbox-tools` |
| `make run` | 构建并运行 `csgclaw serve` |
| `make fmt` | 格式化 Go 源码 |
| `make test` | 运行 `go test ./...` |

可以通过 `SANDBOX_TOOLS_DIR=/path make install-sandbox-cli` 覆盖沙盒 CLI 的安装目录。

## Windows 无 make 时

如果 Windows 环境没有 `make`，可以直接使用 PowerShell 构建脚本：

```powershell
.\scripts\build.cmd build
.\scripts\build.cmd build-server-bin
.\scripts\build.cmd install-sandbox-cli
.\scripts\build.cmd test
```

`build.cmd` 包装器会以当前进程级别的 `-ExecutionPolicy Bypass` 调用
`scripts/build.ps1`，不需要修改整机 PowerShell 执行策略。如果直接调用
PowerShell 脚本，请使用：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 build
```

其中默认的 `build` 会对齐 `make build` 的行为：

1. 构建 Web UI 到 `web/static-dist/`。
2. 构建 `bin/csgclaw.exe` 和宿主平台的 `bin/csgclaw-cli.exe`。
3. 构建 Linux 版 `csgclaw-cli` 到 `~/.csgclaw/sandbox-tools/csgclaw-cli`。

## 运行时镜像

Manager 与 Worker 模板保留不同的内置 workspace，但同一种 runtime 共用一个镜像：

| Runtime | 固定镜像 |
|---|---|
| OpenClaw | `opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260610.2-csgclaw` |
| PicoClaw | `opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.6.10` |

固定引用直接写在内置 `agent.toml` 中，不再包含模板 `version` 字段，不再生成镜像 tag，也不再由 CSGClaw CI 构建这些运行时镜像。

## Web UI

| Target | 说明 |
|---|---|
| `make web-install` | 使用固定 pnpm 工具链安装依赖 |
| `make web-dev` | 启动 Vite 开发服务器 |
| `make build-web` | 构建到 `web/static-dist/` |

修改 Vite 应用前请阅读 [Web 开发文档](web/development.zh.md)。

## 打包与发布

每个正式 `csgclaw` bundle 包含：

```text
csgclaw/
  bin/
    csgclaw[.exe]
    boxlite[.exe]                 # 仅支持的平台
    csgclaw_dir/
      csgclaw-cli                # Linux，CPU 架构与 release 一致
```

安装脚本会把沙盒 CLI 复制到 `~/.csgclaw/sandbox-tools/csgclaw-cli`。为兼容自动升级，运行时启动时也会从已安装 bundle 同步该文件。

| Target | 说明 |
|---|---|
| `make package` | 打包当前平台 |
| `make package-all` | 构建并打包当前平台产物 |
| `make release` | 构建配置的跨平台 release bundle |

发布 CI 使用 `.github/workflows/release.yml` 和 `.gitlab/ci.yml`。GitLab CI 发布 CSGClaw release 产物和 CSGClaw 产品镜像，不再构建 PicoClaw/OpenClaw 运行时镜像。

## 相关文档

- [配置](config.zh.md)
- [架构](architecture.md)
- [Web 开发](web/development.zh.md)
