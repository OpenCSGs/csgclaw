# CSGClaw Windows 与 macOS 发布指南

适用于中国大陆公司通过 Microsoft Store 和官网发布桌面应用。最后核对日期：2026-07-29。

## 1. 推荐方案

| 平台                  | 主要发布方式    | 安装包               | 签名与更新                                      |
| --------------------- | --------------- | -------------------- | ----------------------------------------------- |
| Windows               | Microsoft Store | MSIX                 | Microsoft 认证后重新签名，并负责安装和更新      |
| Windows 企业/离线场景 | 官网            | Squirrel `Setup.exe` | 公司 OV 代码签名证书；应用自己更新              |
| macOS                 | 官网            | DMG                  | Apple Developer ID 签名和公证；ZIP 用于应用更新 |

最短路径是先发布 Windows Microsoft Store 版本和 macOS 官网版本。Windows 官网包可以继续保留，等企业客户或离线分发需要时再购买代码签名证书。

## 2. Windows Microsoft Store

### 2.1 一次性准备

1. 在 [Microsoft Store Developer](https://storedeveloper.microsoft.com/) 注册免费的公司开发者账户。
2. 使用公司主体信息、域名邮箱和 D-U-N-S Number 或营业执照完成验证。
3. 在 Partner Center 选择 `New product` → `MSIX or PWA app`，预留 `CSGClaw` 名称。
4. 从 `Product identity` 复制：
   - `Package/Identity/Name`
   - `Package/Identity/Publisher`
   - `Package/Properties/PublisherDisplayName`
5. 在 Windows 10/11 构建机安装 Go、Node.js、pnpm 和 Windows 11 SDK。项目默认使用 Windows SDK `10.0.26100.0`，也可以通过环境变量指定已经安装的 SDK 版本。

提交 Microsoft Store 的 MSIX 由 Microsoft 在认证后重新签名，因此公司不需要为这个渠道购买 CA 代码签名证书。

### 2.2 本地构建

在 Windows PowerShell 中执行：

```powershell
$env:VERSION = '1.0.0'
$env:CSGCLAW_MSIX_IDENTITY_NAME = '<Package/Identity/Name>'
$env:CSGCLAW_MSIX_PUBLISHER = '<Package/Identity/Publisher>'
$env:CSGCLAW_MSIX_PUBLISHER_DISPLAY_NAME = '<Package/Properties/PublisherDisplayName>'

# 可选：指定构建机已经安装的 Windows SDK 版本
$env:CSGCLAW_MSIX_WINDOWS_KIT_VERSION = '<例如 10.0.26100.0>'

.\scripts\build.cmd desktop-msix
```

生成文件：

```text
desktop/out/make/msix/x64/CSGClaw.msix
```

`desktop-msix` 会依次构建 Web UI、Windows Go backend、CLI、Electron 应用和最终 MSIX。每次发布都要提高 `VERSION`，例如从 `1.0.0` 提高到 `1.0.1`。

### 2.3 测试与首次提交

1. 使用 Windows App Certification Kit 检查 `CSGClaw.msix`。
2. 在干净的 Windows 10/11 环境测试安装、启动、登录和主要功能。
3. 在 Partner Center 填写价格与可用性、属性、年龄分级和商店介绍。
4. 上传 `CSGClaw.msix`，提交认证。
5. 认证通过后，从 Microsoft Store 完成一次真实安装和升级测试。

Forge 本地构建使用开发签名，开发机安装时需要信任对应的开发证书；商店发布版本使用 Microsoft 的正式签名。Store 版本运行时会识别 `process.windowsStore`，更新由 Microsoft Store 管理。

## 3. Windows 官网包

官网 Squirrel 包适合企业客户、离线用户和直接下载链接。公司购买：

```text
DigiCert OV Code Signing Certificate
+ DigiCert KeyLocker 云签名
```

购买入口：[DigiCert 中国官网代码签名证书](https://www.digicert.com/cn/signing/code-signing-certificates)。

证书开通后，在 Windows 发布机安装 KeyLocker Tools 和 Windows SDK SignTool，然后设置供应商提供的签名程序与参数：

```powershell
$env:VERSION = '1.0.0'
$env:CSGCLAW_WINDOWS_SIGN_TOOL = '<SignTool或兼容签名程序路径>'
$env:CSGCLAW_WINDOWS_SIGN_PARAMS = '<DigiCert提供并验证通过的完整参数>'
$env:CSGCLAW_DESKTOP_UPDATE_BASE_URL = 'https://download.example.com/updates'

.\scripts\build.cmd desktop-package
```

发布文件：

```text
desktop/out/make/squirrel.windows/x64/
├── CSGClaw-Desktop-<version>-x64-Setup.exe
├── csgclaw_desktop-<version>-full.nupkg
├── csgclaw_desktop-<version>-delta.nupkg  # 可能生成
└── RELEASES
```

上传 `Setup.exe`、NUPKG，最后上传 `RELEASES`。所有 Windows `EXE`、`DLL`、`NODE`、`Update.exe` 和最终 `Setup.exe` 都应具有有效 Authenticode 签名。

## 4. macOS 官网包

### 4.1 一次性准备

1. 以公司 Organization 加入 Apple Developer Program。
2. 由 Account Holder 创建 `Developer ID Application` 证书。
3. 在发布 Mac 安装证书和私钥，并导出加密 P12 保存到公司密码库。
4. 为公司 Apple Account 创建 app-specific password。

### 4.2 本地构建

```bash
export VERSION='1.0.0'
export CSGCLAW_MACOS_SIGN_IDENTITY='Developer ID Application: <公司英文名称> (<TEAM_ID>)'
export APPLE_ID='<公司Apple Account>'
export APPLE_PASSWORD='<app-specific password>'
export APPLE_TEAM_ID='<TEAM_ID>'
export CSGCLAW_DESKTOP_UPDATE_BASE_URL='https://download.example.com/updates'

# Apple Silicon
make desktop-package TARGET_OS=darwin TARGET_ARCH=arm64

# Intel
make desktop-package TARGET_OS=darwin TARGET_ARCH=amd64
```

Forge 可以从当前钥匙串自动发现有效的 `Developer ID Application` 身份；这里仍显式设置
`CSGCLAW_MACOS_SIGN_IDENTITY`，供后续最终 DMG 签名复用。CI 会在导入 P12 后自动得到这个值，不需要人工配置。

Forge 会签名并公证 APP，然后生成 ZIP 和 DMG。最终 DMG 再完成一次签名、公证和 staple：

```bash
export CSGCLAW_DMG_PATH='<最终DMG路径>'

codesign --force --timestamp \
  --identifier 'com.opencsg.csgclaw.desktop.dmg' \
  --sign "$CSGCLAW_MACOS_SIGN_IDENTITY" \
  "$CSGCLAW_DMG_PATH"

xcrun notarytool submit "$CSGCLAW_DMG_PATH" \
  --apple-id "$APPLE_ID" \
  --password "$APPLE_PASSWORD" \
  --team-id "$APPLE_TEAM_ID" \
  --wait

xcrun stapler staple "$CSGCLAW_DMG_PATH"
xcrun stapler validate "$CSGCLAW_DMG_PATH"
hdiutil verify "$CSGCLAW_DMG_PATH"
```

DMG 用于官网首次安装；ZIP 和 `RELEASES.json` 用于应用自动更新。DMG 的 SHA-256 和文件大小在 staple 完成后计算。

## 5. 未来 CI 方案

当前仓库只实现本地 MSIX 构建，远程 GitHub CI 留作下一阶段。

首次发布仍在 Partner Center 完成公司验证、产品创建、商店资料和年龄分级。以后每周 1～2 次发版可以在 Windows Runner 自动完成：

```text
设置版本号
  → 构建 MSIX
  → 保存构建产物
  → Microsoft Store Developer CLI 上传并提交
  → 查询认证与发布状态
```

未来 CI 的 Repository Variables：

```text
CSGCLAW_MSIX_IDENTITY_NAME
CSGCLAW_MSIX_PUBLISHER
CSGCLAW_MSIX_PUBLISHER_DISPLAY_NAME
CSGCLAW_MSIX_WINDOWS_KIT_VERSION       # 可选
MICROSOFT_STORE_PRODUCT_ID
```

自动提交商店时使用以下 Secrets：

```text
PARTNER_CENTER_TENANT_ID
PARTNER_CENTER_SELLER_ID
PARTNER_CENTER_CLIENT_ID
PARTNER_CENTER_CLIENT_SECRET
```

Microsoft Store Developer CLI 当前是 preview，适合 CSGClaw 这类免费产品自动上传和提交；商店认证由 Microsoft 异步完成。Windows App Certification Kit 需要活动用户会话，安排在 Windows 测试机或自托管 Runner 上运行。

macOS CI 使用 macOS Runner，导入 Developer ID P12 到临时 Keychain，再执行与本地相同的签名、公证和验证命令。
CI 从导入的 P12 自动识别唯一的 `Developer ID Application` 身份，并校验其 Team ID，因此不需要配置
`CSGCLAW_MACOS_SIGN_IDENTITY`。GitHub Actions 需要以下 Repository Secrets：

```text
CSGCLAW_MACOS_CERTIFICATE_P12_BASE64
CSGCLAW_MACOS_CERTIFICATE_PASSWORD
APPLE_ID
APPLE_PASSWORD
APPLE_TEAM_ID
```

证书内容和密码仅提供给临时 Keychain 导入步骤；Apple Account 公证凭据仅提供给 macOS 打包与 DMG 公证步骤。
任何一项缺失都会终止 macOS Release，避免发布临时签名或未公证产物。

## 6. 发布验收

- Microsoft Store：商店安装、启动和升级成功，发布者信息正确。
- Windows 官网：`Get-AuthenticodeSignature` 显示最终 PE 文件和 `Setup.exe` 状态为 `Valid`。
- macOS：`codesign --verify`、`spctl --assess`、`stapler validate` 和 `hdiutil verify` 全部通过。
- 每个平台都从最终公开地址完成一次全新安装和一次版本升级。

## 官方资料

- [Microsoft：Windows 代码签名选项](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options)
- [Microsoft Store 发布入门](https://learn.microsoft.com/en-us/windows/apps/publish/get-started)
- [Partner Center 公司开发者账户](https://learn.microsoft.com/zh-cn/windows/apps/publish/partner-center/open-a-developer-account)
- [Microsoft Store Developer CLI](https://learn.microsoft.com/en-us/windows/apps/publish/msstore-dev-cli/overview)
- [Electron Forge MSIX Maker](https://www.electronforge.io/config/makers/msix)
- [Apple Developer ID 证书](https://developer.apple.com/help/account/certificates/create-developer-id-certificates/)
- [Apple 公证流程](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)
