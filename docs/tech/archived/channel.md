# Channel

除了内置的IM，CSGClaw也支持其他Channel：
- Feishu
- Matrix

## Feishu

当前版块阐述如何实现对Feishu渠道的支持。

### API层面

类似现有的IM路由：
- 创建用户：POST /api/v1/channels/feishu/users
- 获取用户列表：GET /api/v1/channels/feishu/users
- 创建房间：POST /api/v1/channels/feishu/rooms
- 获取房间列表：GET /api/v1/channels/feishu/rooms
- 添加成员：POST /api/v1/channels/feishu/rooms/<room_id>/members
- 获取成员列表：GET /api/v1/channels/feishu/rooms/<room_id>/members

### 核心业务代码

位于 /internal/channel/feishu.go

### CLI能力

- `csgclaw user`: 添加 `-channel` 可选参数，默认为csgclaw，就是当前内置的IM；如果指定为feishu，就走feishu模块的功能
- `csgclaw room`: 添加 `-channel` 可选参数，默认为csgclaw，就是当前内置的IM；如果指定为feishu，就走feishu模块的功能


### 创建房间（真正实现）

修改internal/channel/feishu.go，将创建Room的功能，从Mock实现改为基于以下示例去真正实现：

```go
package main

import (
	"context"
	"fmt"
	"github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// POST /open-apis/im/v1/chats
func main() {
	// 创建 Client
	client := lark.NewClient("appID", "appSecret")
	// 创建请求对象
	req := larkim.NewCreateChatReqBuilder().
		UserIdType("open_id").
		SetBotManager(false).
		Uuid("b13g2t38-1jd2-458b-8djf-dtbca5104204").
		Body(larkim.NewCreateChatReqBodyBuilder().
			Avatar("default-avatar_44ae0ca3-e140-494b-956f-78091e348435").
			Name("测试群名称").
			Description("测试群描述").
			I18nNames(larkim.NewI18nNamesBuilder().Build()).
			OwnerId("4d7a3c6g").
			UserIdList([]string{}).
			BotIdList([]string{}).
			GroupMessageType("chat").
			ChatMode("group").
			ChatType("private").
			JoinMessageVisibility("all_members").
			LeaveMessageVisibility("all_members").
			MembershipApproval("no_approval_required").
			RestrictedModeSetting(larkim.NewRestrictedModeSettingBuilder().Build()).
			UrgentSetting("all_members").
			VideoConferenceSetting("all_members").
			EditPermission("all_members").
			HideMemberCountSetting("all_members").
			Build()).
		Build()
	// 发起请求
	resp, err := client.Im.V1.Chat.Create(context.Background(), req)

	// 处理错误
	if err != nil {
		fmt.Println(err)
		return
	}

	// 服务端错误处理
	if !resp.Success() {
		fmt.Println(resp.Code, resp.Msg, resp.RequestId())
		return
	}

	// 业务处理
	fmt.Println(larkcore.Prettify(resp))
}
```

## 邀请用户进房间

修改internal/channel/feishu.go，将AddRoomMembers的功能，从Mock实现改为基于以下示例去真正实现：

```go
package main

import (
	"context"
	"fmt"
	"github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// SDK 使用文档：https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/server-side-sdk/golang-sdk-guide/preparations
// 复制该 Demo 后, 需要将 "YOUR_APP_ID", "YOUR_APP_SECRET" 替换为自己应用的 APP_ID, APP_SECRET.
// 以下示例代码默认根据文档示例值填充，如果存在代码问题，请在 API 调试台填上相关必要参数后再复制代码使用
func main() {
	// 创建 Client
	client := lark.NewClient("YOUR_APP_ID", "YOUR_APP_SECRET")
	// 创建请求对象
	req := larkim.NewCreateChatMembersReqBuilder().
		ChatId(`oc_a0553eda9014c201e6969b478895c230`).
		MemberIdType(`open_id`).
		SucceedType(0).
		Body(larkim.NewCreateChatMembersReqBodyBuilder().
			IdList([]string{`4d7a3c6g`}).
			Build()).
		Build()

	// 发起请求
	resp, err := client.Im.V1.ChatMembers.Create(context.Background(), req)

	// 处理错误
	if err != nil {
		fmt.Println(err)
		return
	}

	// 服务端错误处理
	if !resp.Success() {
		fmt.Printf("logId: %s, error response: \n%s", resp.RequestId(), larkcore.Prettify(resp.CodeError))
		return
	}

	// 业务处理
	fmt.Println(larkcore.Prettify(resp))
}
```

## 获取房间成员列表

修改internal/channel/feishu.go，将ListRoomMembers的功能，从Mock实现改为基于以下示例去真正实现（不需要专门判断roomID是否存在，larkim的接口自己会报错）：

```go
package main
import (
    "context"
    "fmt"
    "github.com/larksuite/oapi-sdk-go/v3"
    "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// SDK 使用文档：https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/server-side-sdk/golang-sdk-guide/preparations
// 复制该 Demo 后, 需要将 "YOUR_APP_ID", "YOUR_APP_SECRET" 替换为自己应用的 APP_ID, APP_SECRET.
// 以下示例代码默认根据文档示例值填充，如果存在代码问题，请在 API 调试台填上相关必要参数后再复制代码使用
func main(){
   // 创建 Client
   client := lark.NewClient("YOUR_APP_ID", "YOUR_APP_SECRET")
   // 创建请求对象
   req := larkim.NewGetChatMembersReqBuilder().
        ChatId(`oc_a0553eda9014c201e6969b478895c230`).
        MemberIdType(`open_id`).
        PageSize(20

**默认值**：`20`

**数据校验规则**：

- 最大值：`100`).
        PageToken(`WWxHTStrOEs5WHZpNktGbU94bUcvMWlxdDUzTWt1OXNrRmlLaGRNVG0yaz0=`).
       Build()

   // 发起请求
   resp,err := client.Im.V1.ChatMembers.Get(context.Background(),req)


   // 处理错误
    if err != nil {
        fmt.Println(err)
        return
    }

    // 服务端错误处理
    if !resp.Success() {
        fmt.Printf("logId: %s, error response: \n%s", resp.RequestId(), larkcore.Prettify(resp.CodeError))
        return
    }

    // 业务处理
        fmt.Println(larkcore.Prettify(resp))
}
```

## 获取房间列表

修改internal/channel/feishu.go，将ListRooms的功能，从Mock实现改为基于以下示例去真正实现

```go
package main

import (
	"context"
	"fmt"
	"github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// SDK 使用文档：https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/server-side-sdk/golang-sdk-guide/preparations
// 复制该 Demo 后, 需要将 "YOUR_APP_ID", "YOUR_APP_SECRET" 替换为自己应用的 APP_ID, APP_SECRET.
// 以下示例代码默认根据文档示例值填充，如果存在代码问题，请在 API 调试台填上相关必要参数后再复制代码使用
func main() {
	// 创建 Client
	client := lark.NewClient("YOUR_APP_ID", "YOUR_APP_SECRET")
	// 创建请求对象
	req := larkim.NewListChatReqBuilder().
		UserIdType(`open_id`).
		SortType(`ByCreateTimeAsc`).
		PageToken(`dmJCRHhpd3JRbGV1VEVNRFFyTitRWDY5ZFkybmYrMEUwMUFYT0VMMWdENEtuYUhsNUxGMDIwemtvdE5ORjBNQQ==`).
		PageSize(10).
		Build()

	// 发起请求
	resp, err := client.Im.V1.Chat.List(context.Background(), req)

	// 处理错误
	if err != nil {
		fmt.Println(err)
		return
	}

	// 服务端错误处理
	if !resp.Success() {
		fmt.Printf("logId: %s, error response: \n%s", resp.RequestId(), larkcore.Prettify(resp.CodeError))
		return
	}

	// 业务处理
	fmt.Println(larkcore.Prettify(resp))
}
```