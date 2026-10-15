# Embedded browser / 内嵌浏览器

## Desktop

Desktop browser tabs use Electron `WebContentsView`, so websites are top-level Chromium pages rather than iframe documents.
Websites that deny framing can render in the right panel without removing their security headers.
Back, forward, refresh, website popups, URL changes and Escape are connected to the shared chat tab controller.
The application renderer reports the visible content rectangle; the main process clips it to the window and accounts for renderer zoom.
Native pages are hidden underneath application dialogs and menus, and hidden tabs pause media and mute audio.
Closing a tab or reloading the application closes its native WebContents.

Website cookies and storage use a dedicated persistent browser partition, separate from the application session.
Packaged Desktop builds enable Chromium cookie encryption for this browser profile.
Remote pages have no preload or Node integration and cannot call Desktop IPC.
Main-process IPC accepts only the application's main frame and validated HTTP/HTTPS navigation and bounds.
Application-origin navigation, redirects and subresource requests are blocked in the browser partition, and application authorization headers are never injected there.
Automatic downloads without a user gesture and privileged permissions are denied.
HTML file previews continue to use the existing sanitized, isolated document preview.

The browser edition continues to use an isolated iframe and offers external opening when a website refuses embedding.
This capability requires the newly built Desktop shell and preload, not only updated web assets.

## 中文

Desktop 网页标签使用 Electron `WebContentsView`，网页作为独立的 Chromium 顶层页面加载。
因此禁止 iframe 嵌入的网站也能在右侧面板显示，无需删除网站的安全响应头。
前进、后退、刷新、网页弹出链接、地址变化和 Esc 由共享聊天标签控制器承载。
前端上报可见内容区域，主进程按窗口边界裁剪，并处理应用界面缩放。
应用弹层和菜单覆盖页面时隐藏原生视图，隐藏标签暂停媒体并静音。
关闭标签或刷新整个应用会销毁对应的原生 WebContents。

网页 Cookie 和存储使用独立的持久化分区，不与应用登录会话共用。
Desktop 打包版本启用 Chromium Cookie 加密。
外部网页没有 preload、Node 权限或 Desktop IPC 接口。
主进程只接受应用主 frame 的 IPC，并校验网址、命令及尺寸。
浏览器分区禁止访问应用 origin，包括重定向及子资源，且不注入应用 Authorization 头。
没有用户操作触发的自动下载和特权权限请求会被拒绝。
HTML 文件继续沿用已有的清洗及隔离预览。

网页版仍使用隔离 iframe，禁止嵌入的网站通过外部浏览器打开。
此能力需要运行包含新主进程和 preload 的 Desktop 构建，仅更新 Web 页面不足以启用。

## Integrated window chrome / 融合标题栏

Desktop uses a hidden system title bar with a 48px application toolbar and native window controls.
The toolbar provides route history navigation, sidebar visibility and workspace context, with explicit drag and no-drag regions.
The workspace viewport reserves the toolbar height, and native browser bounds continue to follow the visible content rectangle.
macOS keeps its native traffic lights; Windows and Linux use theme-aware Window Controls Overlay.
Application focus changes come from the native window rather than DOM focus, so clicking an embedded website does not mark the application inactive.

Desktop 隐藏默认系统标题栏，使用 48px 的融合应用顶栏，并保留原生窗口控件。
顶栏支持页面前进后退、侧栏开关和工作区上下文，按钮区域明确排除窗口拖动。
工作区高度扣除顶栏高度，原生浏览器视图继续按可见区域更新位置。
macOS 保留原生红黄绿按钮，Windows 和 Linux 使用跟随主题的原生窗口控制覆盖层。
应用激活状态来自原生窗口事件，因此点击内嵌网页不会把整个应用误判为未激活。
