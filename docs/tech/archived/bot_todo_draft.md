根据以下内容更新和完善docs/architecture.md（先不要改代码）:
- 增加bot的概念,可以是manager或worker
- 一个bot底层对应一个agent和一个IM（或Channel,如feishu）中的user
- 新增csgclaw bot <list|create> -channel命令，-channel默认为csgclaw，也可以是feishu
- 新增/api/v1/bots路由,可以POST创建或者GET获取列表，对应的业务逻辑放到/internal/bot里面
- POST创建,会做两件事情：1.创建一个agent（会创建对应的manager或worker的box），2.针对csgclaw的IM（或feishu的channel），会创建一个user
- 代码可以部分参考 `/api/v1/agents`（只处理了 csgclaw IM 的部分，而且）
- 统一创建bot的CLI和API,支持csgclaw和feishu两种channel
