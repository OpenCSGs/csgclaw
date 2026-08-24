package enginetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel/feishu"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// feishuAdapterHarness is deliberately test-only. It accepts only the Engine
// contract and the existing channel credential owner.
type feishuAdapterHarness struct {
	engine      agentengine.Interface
	credentials feishu.AgentCredentialProvider
}

func (h feishuAdapterHarness) Ingress(ctx context.Context, agentID string, key agentengine.ConversationKey, eventID, text string) agentengine.TurnResult {
	_, app, ok := h.credentials.BotConfigForAgent(agentID)
	if !ok || strings.TrimSpace(app.AppID) == "" || strings.TrimSpace(app.AppSecret) == "" {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorAgentUnavailable, Message: "Feishu binding is unavailable"}}
	}
	return h.engine.Conversations(agentID).Run(ctx, agentengine.TurnRequest{
		ID: agentengine.TurnID(eventID), ConversationKey: key,
		Admission: agentengine.AdmissionSupersede, Interaction: agentengine.InteractionSkipUserInput,
		Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: text}},
	}, nil)
}

type feishuCredentialStub struct {
	app feishu.AppConfig
}

func (p feishuCredentialStub) BotConfigForAgent(string) (string, feishu.AppConfig, bool) {
	return "participant-feishu", p.app, true
}

func TestFeishuAdapterHarnessUsesEngineWithoutLeakingChannelSecrets(t *testing.T) {
	seed := runningAgent("agent-feishu")
	client := NewMemoryClient(seed)
	client.SetTurnBehavior(func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: request.Input[0].Text, Dispatched: true}
	})
	const appID = "cli_feishu_only"
	const appSecret = "feishu-secret-only"
	harness := feishuAdapterHarness{
		engine:      client,
		credentials: feishuCredentialStub{app: feishu.AppConfig{AppID: appID, AppSecret: appSecret}},
	}
	result := harness.Ingress(context.Background(), seed.ID, "chat-1", "event-1", "hello from Feishu")
	if result.Status != agentengine.TurnSucceeded {
		t.Fatalf("result = %+v", result)
	}
	retried := harness.Ingress(context.Background(), seed.ID, "chat-1", "event-1", "hello from Feishu")
	if retried.Status != agentengine.TurnSucceeded || retried.Output != result.Output {
		t.Fatalf("retried result = %+v", retried)
	}
	calls := client.Calls()
	if len(calls) != 1 || calls[0].AgentID != seed.ID || calls[0].Request.ConversationKey != "chat-1" || calls[0].Request.ID != "event-1" || calls[0].Request.Admission != agentengine.AdmissionSupersede {
		t.Fatalf("Engine calls = %+v", calls)
	}
	agentItem, err := client.Agents().Get(context.Background(), seed.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(struct {
		Agent  agentengine.Agent
		Calls  []TurnCall
		Result agentengine.TurnResult
	}{Agent: agentItem, Calls: calls, Result: result})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), appID) || strings.Contains(string(encoded), appSecret) {
		t.Fatalf("Feishu channel credentials leaked into Engine data: %s", encoded)
	}
}

type feishuDeliveryHarness struct {
	engine agentengine.Interface
	client *lark.Client
}

func (h feishuDeliveryHarness) IngressAndDeliver(ctx context.Context, agentID, roomID string) agentengine.TurnResult {
	var links []string
	result := h.engine.Conversations(agentID).Run(ctx, agentengine.TurnRequest{
		ID: "feishu-event-file", ConversationKey: agentengine.ConversationKey(roomID),
		Admission: agentengine.AdmissionSupersede, Interaction: agentengine.InteractionSkipUserInput,
		Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: "create files"}},
	}, agentengine.EventSinkFunc(func(ctx context.Context, event agentengine.TurnEvent) error {
		if event.Output == nil {
			return nil
		}
		switch event.Output.Kind {
		case agentengine.OutputItemResourceLink:
			link, ok := event.Output.Payload.(activity.ResourceLink)
			if !ok {
				return fmt.Errorf("resource link output has invalid payload")
			}
			links = append(links, fmt.Sprintf("[%s](<%s>)", link.Name, link.URI))
		}
		return nil
	}))
	if result.Status != agentengine.TurnSucceeded {
		return result
	}
	for _, file := range result.Files {
		if err := h.uploadAndSend(ctx, agentID, roomID, file); err != nil {
			return agentengine.TurnResult{Status: agentengine.TurnFailed, Dispatched: result.Dispatched, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
		}
	}
	if len(links) > 0 {
		content, _ := json.Marshal(map[string]any{
			"config":   map[string]bool{"wide_screen_mode": true},
			"elements": []map[string]string{{"tag": "markdown", "content": strings.Join(links, "\n")}},
		})
		if err := h.send(ctx, roomID, larkim.MsgTypeInteractive, string(content), "runtime-links"); err != nil {
			return agentengine.TurnResult{Status: agentengine.TurnFailed, Dispatched: result.Dispatched, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
		}
	}
	return result
}

func (h feishuDeliveryHarness) uploadAndSend(ctx context.Context, agentID, roomID string, file agentengine.OutputFile) error {
	download, err := h.engine.Conversations(agentID).Files().Get(ctx, file.ID)
	if err != nil {
		return err
	}
	defer download.Content.Close()
	metadata := download.Metadata
	if strings.HasPrefix(metadata.MediaType, "image/") {
		response, err := h.client.Im.V1.Image.Create(ctx, larkim.NewCreateImageReqBuilder().
			Body(larkim.NewCreateImageReqBodyBuilder().ImageType("message").Image(download.Content).Build()).Build())
		if err != nil {
			return err
		}
		if !response.Success() || response.Data == nil || strings.TrimSpace(larkcore.StringValue(response.Data.ImageKey)) == "" {
			return fmt.Errorf("Feishu image upload failed: code=%d msg=%s", response.Code, response.Msg)
		}
		content, _ := json.Marshal(map[string]string{"image_key": larkcore.StringValue(response.Data.ImageKey)})
		return h.send(ctx, roomID, larkim.MsgTypeImage, string(content), metadata.ID)
	}
	response, err := h.client.Im.V1.File.Create(ctx, larkim.NewCreateFileReqBuilder().
		Body(larkim.NewCreateFileReqBodyBuilder().FileType("stream").FileName(metadata.Name).File(download.Content).Build()).Build())
	if err != nil {
		return err
	}
	if !response.Success() || response.Data == nil || strings.TrimSpace(larkcore.StringValue(response.Data.FileKey)) == "" {
		return fmt.Errorf("Feishu file upload failed: code=%d msg=%s", response.Code, response.Msg)
	}
	content, _ := json.Marshal(map[string]string{"file_key": larkcore.StringValue(response.Data.FileKey)})
	return h.send(ctx, roomID, larkim.MsgTypeFile, string(content), metadata.ID)
}

func (h feishuDeliveryHarness) send(ctx context.Context, roomID, messageType, content, uuid string) error {
	response, err := h.client.Im.V1.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().ReceiveId(roomID).MsgType(messageType).Content(content).Uuid(uuid).Build()).Build())
	if err != nil {
		return err
	}
	if !response.Success() || response.Data == nil || strings.TrimSpace(larkcore.StringValue(response.Data.MessageId)) == "" {
		return fmt.Errorf("Feishu message send failed: code=%d msg=%s", response.Code, response.Msg)
	}
	return nil
}

func TestFeishuAdapterUploadsOnlyAuthorizedEngineFilesThroughOpenAPI(t *testing.T) {
	t.Parallel()

	imageContent := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	fileContent := []byte("%PDF-1.7\ncontract report\n")
	var mu sync.Mutex
	var calls []string
	resourceLinkFetched := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			writeFeishuJSON(w, map[string]any{"code": 0, "msg": "ok", "tenant_access_token": "tenant-test", "expire": 7200})
			return
		}
		if r.URL.Path == "/forbidden-resource-link" {
			mu.Lock()
			resourceLinkFetched = true
			mu.Unlock()
			http.Error(w, "resource_link must not be downloaded", http.StatusInternalServerError)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tenant-test" {
			http.Error(w, "missing tenant token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/open-apis/im/v1/images":
			if err := assertMultipartFile(r, "image", imageContent); err != nil || r.FormValue("image_type") != "message" {
				http.Error(w, fmt.Sprintf("invalid image upload: %v", err), http.StatusBadRequest)
				return
			}
			mu.Lock()
			calls = append(calls, "upload:image")
			mu.Unlock()
			writeFeishuJSON(w, map[string]any{"code": 0, "msg": "ok", "data": map[string]string{"image_key": "img-test"}})
		case "/open-apis/im/v1/files":
			if err := assertMultipartFile(r, "file", fileContent); err != nil || r.FormValue("file_type") != "stream" || r.FormValue("file_name") != "report.pdf" {
				http.Error(w, fmt.Sprintf("invalid file upload: %v", err), http.StatusBadRequest)
				return
			}
			mu.Lock()
			calls = append(calls, "upload:file")
			mu.Unlock()
			writeFeishuJSON(w, map[string]any{"code": 0, "msg": "ok", "data": map[string]string{"file_key": "file-test"}})
		case "/open-apis/im/v1/messages":
			var body struct {
				ReceiveID string `json:"receive_id"`
				MsgType   string `json:"msg_type"`
				Content   string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ReceiveID != "chat-test" || body.Content == "" || r.URL.Query().Get("receive_id_type") != "chat_id" {
				http.Error(w, fmt.Sprintf("invalid message: %v", err), http.StatusBadRequest)
				return
			}
			mu.Lock()
			calls = append(calls, "send:"+body.MsgType)
			mu.Unlock()
			writeFeishuJSON(w, map[string]any{"code": 0, "msg": "ok", "data": map[string]string{"message_id": "message-test"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	seed := runningAgent("agent-feishu-files")
	client := NewMemoryClient(seed)
	image := memoryOutputFile(t, client, seed.ID, "result.png", "image/png", imageContent)
	report := memoryOutputFile(t, client, seed.ID, "report.pdf", "application/pdf", fileContent)
	client.SetTurnBehavior(func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventOutputItem, Output: &agentengine.OutputItem{
			Kind: agentengine.OutputItemResourceLink, Payload: activity.ResourceLink{Type: "resource_link", Name: "docs", URI: server.URL + "/forbidden-resource-link"},
		}}); err != nil {
			return agentengine.TurnResult{Status: agentengine.TurnFailed, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Files: []agentengine.OutputFile{image, report}, Dispatched: true}
	})
	harness := feishuDeliveryHarness{
		engine: client,
		client: lark.NewClient("app-test", "secret-test", lark.WithOpenBaseUrl(server.URL)),
	}
	result := harness.IngressAndDeliver(context.Background(), seed.ID, "chat-test")
	if result.Status != agentengine.TurnSucceeded || len(result.Files) != 2 || !result.Dispatched {
		t.Fatalf("delivery result = %+v", result)
	}
	if result.Files[0].ID == result.Files[1].ID || !strings.HasPrefix(result.Files[0].ID, "file-") || !strings.HasPrefix(result.Files[1].ID, "file-") {
		t.Fatalf("delivery file IDs = %q, %q", result.Files[0].ID, result.Files[1].ID)
	}
	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	linkFetched := resourceLinkFetched
	mu.Unlock()
	wantCalls := []string{"upload:image", "send:image", "upload:file", "send:file", "send:interactive"}
	if fmt.Sprint(gotCalls) != fmt.Sprint(wantCalls) {
		t.Fatalf("Feishu OpenAPI calls = %v, want %v", gotCalls, wantCalls)
	}
	if linkFetched {
		t.Fatal("ordinary resource_link was downloaded")
	}
}

func TestFeishuAdapterDoesNotUploadFilesFromFailedTurn(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected Feishu request", http.StatusInternalServerError)
	}))
	defer server.Close()

	seed := runningAgent("agent-feishu-failed-file")
	client := NewMemoryClient(seed)
	file := memoryOutputFile(t, client, seed.ID, "failed.pdf", "application/pdf", []byte("failed turn output"))
	client.SetTurnBehavior(func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		return agentengine.TurnResult{
			Status: agentengine.TurnFailed, Files: []agentengine.OutputFile{file}, Dispatched: true,
			Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "turn failed"},
		}
	})
	harness := feishuDeliveryHarness{
		engine: client,
		client: lark.NewClient("app-test", "secret-test", lark.WithOpenBaseUrl(server.URL)),
	}
	result := harness.IngressAndDeliver(context.Background(), seed.ID, "chat-test")
	if result.Status != agentengine.TurnFailed || len(result.Files) != 0 || requests.Load() != 0 {
		t.Fatalf("result=%+v Feishu requests=%d", result, requests.Load())
	}
}

func memoryOutputFile(t testing.TB, client agentengine.Interface, agentID, name, mediaType string, content []byte) agentengine.OutputFile {
	t.Helper()
	digest := sha256.Sum256(content)
	file, err := client.Conversations(agentID).Files().Create(context.Background(), agentengine.FileCreateRequest{
		Name: name, MIMEType: mediaType, SizeBytes: int64(len(content)), SHA256: hex.EncodeToString(digest[:]),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func assertMultipartFile(r *http.Request, field string, want []byte) error {
	if err := r.ParseMultipartForm(int64(len(want) + 1<<20)); err != nil {
		return err
	}
	file, _, err := r.FormFile(field)
	if err != nil {
		return err
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s bytes = %d, want %d", field, len(got), len(want))
	}
	return nil
}

func writeFeishuJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
