package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

func TestConversationResourceHTTP(t *testing.T) {
	for _, kind := range []string{agent.RuntimeKindCodex, agent.RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			item := completeWorkerAgent("agent-resources", "Resources")
			item.RuntimeKind, item.RuntimeName = kind, kind
			item.Status, item.DesiredState = "stopped", "stopped"
			item.MCPServers = map[string]any{"search": map[string]any{"url": "https://fixture.invalid/mcp", "display_name": "研发搜索"}}
			rt := &resourceEnablementRuntime{snapshotMCPServersRuntime: snapshotMCPServersRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: kind}}}
			controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, agent.WithRuntime(rt))
			layout, err := controller.AgentLayout(item.ID)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(layout.SkillsRoot, "review")
			if err = os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review\ndescription: review fixture\n---\n"), 0600); err != nil {
				t.Fatal(err)
			}
			state := im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin", Name: "Admin", Role: "admin"}, {ID: item.ID, Name: "Resources", Role: "worker"}}, Rooms: []im.Room{{ID: "dm", IsDirect: true, Members: []string{"user-admin", item.ID}}, {ID: "group", Members: []string{"user-admin", item.ID}}}}
			statePath := filepath.Join(t.TempDir(), "im.json")
			if err = im.SaveBootstrap(statePath, state); err != nil {
				t.Fatal(err)
			}
			messages, err := im.NewServiceFromPath(statePath)
			if err != nil {
				t.Fatal(err)
			}
			engine := agentengine.New(controller)
			bridge := im.NewParticipantBridge("")
			h := NewHandler(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, messages, nil, bridge, nil, nil)
			h.SetParticipantService(participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "pt-resources", AgentID: item.ID, Channel: "csgclaw", Type: participant.TypeAgent, ChannelUserRef: item.ID, ChannelUserKind: participant.ChannelUserKindLocalUserID}}), participant.WithAgentEngine(engine)))
			routes := h.Routes()
			post := func(payload string) *httptest.ResponseRecorder {
				t.Helper()
				w := httptest.NewRecorder()
				routes.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(payload)))
				return w
			}
			get := httptest.NewRecorder()
			routes.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-resources/resources?type=skill&pagination=cursor", nil))
			if get.Code != 200 || !strings.Contains(get.Body.String(), `"id":"review"`) {
				t.Fatalf("资源查询失败：%d %s", get.Code, get.Body)
			}
			for _, query := range []string{"type=skill", "type=other&pagination=cursor", "type=skill&pagination=cursor&page=1", "type=skill&pagination=page&cursor=old"} {
				response := httptest.NewRecorder()
				routes.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-resources/resources?"+query, nil))
				if response.Code != 400 || !strings.Contains(response.Body.String(), "invalid_resource_query") {
					t.Fatalf("无效分页被接受：%d %s", response.Code, response.Body)
				}
			}
			payload := `{"room_id":"dm","sender_id":"user-admin","client_message_id":"skill-send","content":"","resource_refs":[{"source":"skill","id":"review"}],"metadata":{"csgclaw":{"other":"kept","resource_selection":{"version":1,"agent_id":"forged"}}}}`
			response := post(payload)
			if response.Code != 201 {
				t.Fatalf("单个技能发送失败：%d %s", response.Code, response.Body)
			}
			var message apitypes.Message
			if err = json.Unmarshal(response.Body.Bytes(), &message); err != nil {
				t.Fatal(err)
			}
			selection := apitypes.ResourceSelectionFromMetadata(message.Metadata)
			if message.Content != "" || selection == nil || selection.AgentID != item.ID || !selection.SingleSkill() {
				t.Fatalf("选择快照错误：%+v", message)
			}
			body, prompt, err := bridge.ResourceInput(t.Context(), item.ID, message.Content, selection)
			if err != nil || body != "" || !strings.Contains(prompt, `"source":"skill"`) {
				t.Fatal("执行输入失败", body, prompt, err)
			}
			room, _ := messages.Room("dm")
			sender, _ := messages.User("user-admin")
			events, unsubscribe := bridge.Subscribe("pt-resources")
			defer unsubscribe()
			bridge.EnqueueMessageEvent(room, sender, message, "pt-resources")
			select {
			case event := <-events:
				if event.ResourceSelection == nil || event.ResourceSelection.AgentID != item.ID {
					t.Fatal("事件丢失引用", event)
				}
			default:
				t.Fatal("技能事件未投递")
			}
			for _, invalid := range []string{
				`{"room_id":"dm","sender_id":"user-admin","content":"<slash-command name=\"new\" arg=\"conversation\"></slash-command>","resource_refs":[{"source":"skill","id":"review"}]}`,
				`{"room_id":"dm","sender_id":"user-admin","content":"","metadata":{"csgclaw":{"resource_selection":{"version":1,"agent_id":"agent-resources","refs":[{"source":"skill","id":"review"}]}}},"ResourceSelection":{"version":1}}`,
				`{"room_id":"dm","sender_id":"user-admin","content":"","resource_refs":[{"source":"mcp","id":"search"}]}`,
				`{"room_id":"dm","sender_id":"user-admin","content":"正文","resource_refs":[{"source":"mcp","id":"missing"}]}`,
				`{"room_id":"group","sender_id":"user-admin","content":"正文","resource_refs":[{"source":"skill","id":"review"}]}`,
			} {
				if got := post(invalid); got.Code < 400 {
					t.Fatalf("无效输入被接受：%s", got.Body)
				}
			}
			reply := post(`{"room_id":"dm","sender_id":"user-admin","content":"补充任务","resource_refs":[{"source":"skill","id":"review"}],"relates_to":{"rel_type":"m.thread","event_id":"` + message.ID + `"}}`)
			if reply.Code != 201 || !strings.Contains(reply.Body.String(), "resource_selection") {
				t.Fatalf("讨论消息丢失资源：%d %s", reply.Code, reply.Body)
			}
			legacy := post(`{"room_id":"dm","sender_id":"user-admin","content":"<slash-command name=\"use-skill\" arg=\"review\"></slash-command> 检查"}`)
			if legacy.Code != 201 || strings.Contains(legacy.Body.String(), "slash-command") || !strings.Contains(legacy.Body.String(), "resource_selection") {
				t.Fatalf("历史客户端转换失败：%d %s", legacy.Code, legacy.Body)
			}
			group := post(`{"room_id":"group","sender_id":"user-admin","content":"<slash-command name=\"use-skill\" arg=\"review\"></slash-command> 检查"}`)
			if group.Code != 201 || !strings.Contains(group.Body.String(), "slash-command") {
				t.Fatalf("群聊命令兼容失败：%d %s", group.Code, group.Body)
			}
			var upload bytes.Buffer
			form := multipart.NewWriter(&upload)
			_ = form.WriteField("payload", `{"room_id":"dm","sender_id":"user-admin","content":"检查附件","resource_refs":[{"source":"mcp","id":"search"}]}`)
			part, _ := form.CreateFormFile("files", "fixture.txt")
			_, _ = part.Write([]byte("fixture"))
			_ = form.Close()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", &upload)
			req.Header.Set("Content-Type", form.FormDataContentType())
			uploaded := httptest.NewRecorder()
			routes.ServeHTTP(uploaded, req)
			if uploaded.Code != 201 || !strings.Contains(uploaded.Body.String(), "resource_selection") {
				t.Fatalf("附件引用失败：%d %s", uploaded.Code, uploaded.Body)
			}
			var attachmentMessage apitypes.Message
			if err = json.Unmarshal(uploaded.Body.Bytes(), &attachmentMessage); err != nil || len(attachmentMessage.Attachments) != 1 {
				t.Fatalf("attachment fixture: %+v %v", attachmentMessage, err)
			}
			referenced := post(`{"room_id":"dm","sender_id":"user-admin","content":"","resource_refs":[{"source":"mcp","id":"search"}],"file_references":[{"source":"attachment","attachment_id":"` + attachmentMessage.Attachments[0].ID + `"}]}`)
			var referencedMessage apitypes.Message
			if err = json.Unmarshal(referenced.Body.Bytes(), &referencedMessage); err != nil || referenced.Code != http.StatusCreated {
				t.Fatalf("resource with referenced attachment: %d %s %v", referenced.Code, referenced.Body, err)
			}
			if len(referencedMessage.Attachments) != 1 || apitypes.ResourceSelectionFromMetadata(referencedMessage.Metadata) == nil {
				t.Fatalf("resource or attachment missing: %+v", referencedMessage)
			}
			if err = os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			retry := post(payload)
			if retry.Code != 201 || !strings.Contains(retry.Body.String(), message.ID) {
				t.Fatalf("重试未返回原消息：%d %s", retry.Code, retry.Body)
			}
			if _, _, err = bridge.ResourceInput(t.Context(), item.ID, message.Content, selection); err == nil {
				t.Fatal("执行前未检查移除的技能")
			}
		})
	}
}
