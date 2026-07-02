package feishu

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/im"
	"csgclaw/internal/slashcommand"
	"csgclaw/internal/utils"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

const (
	feishuManagerBotID = "manager"
)

type CreateUserRequest struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
	Avatar string `json:"avatar,omitempty"`
}

type BotInfo struct {
	OpenID  string
	AppName string
}

type CreateChatRequest struct {
	Title        string
	Description  string
	CreatorID    string
	MemberAppIDs []string
}

type CreateChatResponse struct {
	ChatID      string
	Name        string
	Description string
}

type CreateChatFunc func(context.Context, AppConfig, CreateChatRequest) (CreateChatResponse, error)

type AddChatMembersRequest struct {
	ChatID       string
	MemberBotIDs []string
	MemberAppIDs []string
}

type AddChatMembersFunc func(context.Context, AppConfig, AddChatMembersRequest) error

type ListChatMembersFunc func(context.Context, AppConfig, map[string]AppConfig, string) ([]im.User, error)

type ListChatsFunc func(context.Context, AppConfig) ([]im.Room, error)

type ListRoomMessagesFunc func(context.Context, AppConfig, string) ([]im.Message, error)

type DeleteChatFunc func(context.Context, AppConfig, string) error

type SendMessageRequest struct {
	ChatID           string
	Content          string
	UUID             string
	ThreadRootID     string
	MentionID        string
	MentionOpenID    string
	MentionAppConfig AppConfig
}

type SendMessageResponse struct {
	MessageID     string
	SenderOpenID  string
	MentionOpenID string
}

type SendMessageFunc func(context.Context, AppConfig, SendMessageRequest) (SendMessageResponse, error)

type UpdateMessageRequest struct {
	RoomID    string
	SenderID  string
	MessageID string
	Content   string
}

type UpdateMessageResponse struct {
	MessageID string
}

type UpdateMessageFunc func(context.Context, AppConfig, UpdateMessageRequest) (UpdateMessageResponse, error)

type CreateMessageReactionRequest struct {
	SenderID  string
	MessageID string
	EmojiType string
}

type CreateMessageReactionResponse struct {
	ReactionID string
}

type CreateMessageReactionFunc func(context.Context, AppConfig, CreateMessageReactionRequest) (CreateMessageReactionResponse, error)

type DeleteMessageReactionRequest struct {
	SenderID   string
	MessageID  string
	ReactionID string
}

type DeleteMessageReactionFunc func(context.Context, AppConfig, DeleteMessageReactionRequest) error

type Service struct {
	mu                    sync.RWMutex
	users                 map[string]im.User
	byName                map[string]string
	rooms                 map[string]*im.Room
	apps                  map[string]AppConfig
	resolveBotInfo        func(context.Context, AppConfig) (BotInfo, error)
	createChat            CreateChatFunc
	addChatMembers        AddChatMembersFunc
	listChatMembers       ListChatMembersFunc
	listChats             ListChatsFunc
	listRoomMessages      ListRoomMessagesFunc
	deleteChat            DeleteChatFunc
	sendMessage           SendMessageFunc
	updateMessage         UpdateMessageFunc
	createMessageReaction CreateMessageReactionFunc
	deleteMessageReaction DeleteMessageReactionFunc
	messageBus            *MessageBus
	configProvider        Provider
}

func NewService(apps ...map[string]AppConfig) *Service {
	configuredApps := make(map[string]AppConfig)
	if len(apps) > 0 {
		for name, app := range apps[0] {
			configuredApps[name] = app
		}
	}
	return &Service{
		users:                 make(map[string]im.User),
		byName:                make(map[string]string),
		rooms:                 make(map[string]*im.Room),
		apps:                  configuredApps,
		resolveBotInfo:        fetchBotInfo,
		createChat:            defaultCreateChat,
		addChatMembers:        defaultAddChatMembers,
		listChatMembers:       defaultListChatMembers,
		listChats:             defaultListChats,
		listRoomMessages:      defaultListRoomMessages,
		deleteChat:            defaultDeleteChat,
		sendMessage:           defaultSendMessage,
		updateMessage:         defaultUpdateMessage,
		createMessageReaction: defaultCreateMessageReaction,
		deleteMessageReaction: defaultDeleteMessageReaction,
		messageBus:            NewMessageBus(),
	}
}

func NewServiceWithProvider(provider Provider) *Service {
	svc := NewService()
	svc.SetConfigProvider(provider)
	return svc
}

func NewServiceWithBotOpenIDResolver(apps map[string]AppConfig, resolveBotInfo func(context.Context, AppConfig) (BotInfo, error)) *Service {
	svc := NewService(apps)
	svc.SetBotOpenIDResolver(resolveBotInfo)
	return svc
}

func (s *Service) SetBotOpenIDResolver(resolveBotInfo func(context.Context, AppConfig) (BotInfo, error)) {
	if s == nil || resolveBotInfo == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolveBotInfo = resolveBotInfo
}

func NewServiceWithCreateChat(apps map[string]AppConfig, createChat CreateChatFunc) *Service {
	svc := NewService(apps)
	if createChat != nil {
		svc.createChat = createChat
	}
	return svc
}

func NewServiceWithCreateChatAndAddMembers(apps map[string]AppConfig, createChat CreateChatFunc, addChatMembers AddChatMembersFunc, listChatMembers ...ListChatMembersFunc) *Service {
	svc := NewServiceWithCreateChat(apps, createChat)
	if addChatMembers != nil {
		svc.addChatMembers = addChatMembers
	}
	if len(listChatMembers) > 0 && listChatMembers[0] != nil {
		svc.listChatMembers = listChatMembers[0]
	}
	return svc
}

func NewServiceWithListRoomMessages(apps map[string]AppConfig, listRoomMessages ListRoomMessagesFunc) *Service {
	svc := NewService(apps)
	if listRoomMessages != nil {
		svc.listRoomMessages = listRoomMessages
	}
	return svc
}

func NewServiceWithDeleteChat(apps map[string]AppConfig, deleteChat DeleteChatFunc) *Service {
	svc := NewService(apps)
	if deleteChat != nil {
		svc.deleteChat = deleteChat
	}
	return svc
}

func NewServiceWithCreateChatAndListRoomMessages(apps map[string]AppConfig, createChat CreateChatFunc, listRoomMessages ListRoomMessagesFunc) *Service {
	svc := NewServiceWithCreateChat(apps, createChat)
	if listRoomMessages != nil {
		svc.listRoomMessages = listRoomMessages
	}
	return svc
}

func NewServiceWithSendMessage(apps map[string]AppConfig, sendMessage SendMessageFunc) *Service {
	svc := NewService(apps)
	if sendMessage != nil {
		svc.sendMessage = sendMessage
	}
	return svc
}

func NewServiceWithUpdateMessage(apps map[string]AppConfig, updateMessage UpdateMessageFunc) *Service {
	svc := NewService(apps)
	if updateMessage != nil {
		svc.updateMessage = updateMessage
	}
	return svc
}

func NewServiceWithMessageReaction(apps map[string]AppConfig, createMessageReaction CreateMessageReactionFunc, deleteMessageReaction DeleteMessageReactionFunc) *Service {
	svc := NewService(apps)
	if createMessageReaction != nil {
		svc.createMessageReaction = createMessageReaction
	}
	if deleteMessageReaction != nil {
		svc.deleteMessageReaction = deleteMessageReaction
	}
	return svc
}

func (s *Service) MessageBus() *MessageBus {
	if s == nil {
		return nil
	}
	return s.messageBus
}

func (s *Service) AppConfigs() map[string]AppConfig {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	provider := s.configProvider
	apps := cloneAppConfigs(s.apps)
	s.mu.RUnlock()

	if provider != nil {
		return AppsFromSnapshot(provider.Snapshot())
	}
	return apps
}

func (s *Service) SetAppConfigs(apps map[string]AppConfig) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apps = cloneAppConfigs(apps)
}

func cloneAppConfigs(apps map[string]AppConfig) map[string]AppConfig {
	cloned := make(map[string]AppConfig, len(apps))
	for name, app := range apps {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		cloned[name] = app
	}
	return cloned
}

func (s *Service) CreateUser(req CreateUserRequest) (im.User, error) {
	// Mock implementation. Real Feishu support should call the external Feishu API.
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return im.User{}, fmt.Errorf("name is required")
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = fmt.Sprintf("fsu-%d", time.Now().UnixNano())
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = "member"
	}
	avatar := strings.TrimSpace(req.Avatar)
	if avatar == "" {
		avatar = initials(name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[id]; ok {
		return im.User{}, fmt.Errorf("user already exists")
	}
	if existingID, ok := s.byName[strings.ToLower(name)]; ok && existingID != id {
		return im.User{}, fmt.Errorf("name %q already exists", name)
	}

	user := im.User{
		ID:        id,
		Name:      name,
		Role:      role,
		Avatar:    avatar,
		IsOnline:  true,
		AccentHex: accentHexForID(id),
		CreatedAt: time.Now().UTC(),
	}
	s.users[id] = user
	s.byName[strings.ToLower(name)] = id
	return user, nil
}

func (s *Service) ListUsers() []im.User {
	apps := s.AppConfigs()
	s.mu.RLock()
	localUsers := make(map[string]im.User, len(s.users))
	for id, user := range s.users {
		localUsers[id] = user
	}
	resolveBotInfo := s.resolveBotInfo
	s.mu.RUnlock()

	users := make([]im.User, 0, len(apps)+len(localUsers))
	seenIDs := make(map[string]struct{}, len(apps)+len(localUsers))
	configuredBotIDs := make(map[string]struct{}, len(apps))
	for botID, rawApp := range apps {
		configuredBotIDs[botID] = struct{}{}

		app, err := validateAppConfig(rawApp, botID)
		if err != nil {
			continue
		}
		botInfo, err := resolveBotInfo(context.Background(), app)
		if err != nil {
			continue
		}
		openID := strings.TrimSpace(botInfo.OpenID)
		if openID == "" {
			continue
		}
		if _, ok := seenIDs[openID]; ok {
			continue
		}

		user, ok := localUsers[botID]
		if !ok {
			name := strings.TrimSpace(botInfo.AppName)
			if name == "" {
				name = strings.TrimSpace(botID)
			}
			if name == "" {
				name = openID
			}
			user = im.User{
				Name:      name,
				Role:      "member",
				Avatar:    initials(name),
				IsOnline:  true,
				CreatedAt: time.Now().UTC(),
			}
		}
		user.ID = openID
		user.AccentHex = accentHexForID(openID)
		users = append(users, user)
		seenIDs[openID] = struct{}{}
	}
	for id, user := range localUsers {
		if _, ok := configuredBotIDs[id]; ok {
			continue
		}
		if _, ok := seenIDs[user.ID]; ok {
			continue
		}
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b im.User) int { return strings.Compare(a.Name, b.Name) })
	return users
}

func (s *Service) DeleteUser(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return fmt.Errorf("user not found")
	}

	delete(s.users, userID)
	delete(s.byName, strings.ToLower(strings.TrimSpace(user.Name)))

	for id, room := range s.rooms {
		members := make([]string, 0, len(room.Members))
		for _, memberID := range room.Members {
			if memberID != userID {
				members = append(members, memberID)
			}
		}

		messages := make([]im.Message, 0, len(room.Messages))
		for _, message := range room.Messages {
			if message.SenderID != userID {
				messages = append(messages, message)
			}
		}

		if len(members) < 2 {
			delete(s.rooms, id)
			continue
		}

		room.Members = members
		room.Messages = messages
		room.Subtitle = formatMembers(len(members))
	}

	return nil
}

func (s *Service) ResolveBotUser(ctx context.Context, botID string) (im.User, bool, error) {
	if s == nil {
		return im.User{}, false, nil
	}
	openID, _, err := s.ResolveBotOpenID(ctx, botID)
	if err != nil {
		return im.User{}, false, err
	}
	openID = strings.TrimSpace(openID)
	if openID == "" || openID == strings.TrimSpace(botID) {
		return im.User{}, false, nil
	}
	if user, ok := findUserByID(s.ListUsers(), openID); ok {
		return user, true, nil
	}
	name := strings.TrimSpace(botID)
	if name == "" {
		name = openID
	}
	return im.User{
		ID:        openID,
		Name:      name,
		Role:      "member",
		Avatar:    initials(name),
		IsOnline:  true,
		AccentHex: accentHexForID(openID),
		CreatedAt: time.Now().UTC(),
	}, true, nil
}

func (s *Service) EnsureUser(req CreateUserRequest) (im.User, error) {
	if user, ok, err := s.ResolveBotUser(context.Background(), req.ID); err == nil && ok {
		return user, nil
	}
	if user, ok := findUserByID(s.ListUsers(), req.ID); ok {
		return user, nil
	}
	return s.CreateUser(req)
}

func findUserByID(users []im.User, id string) (im.User, bool) {
	id = strings.TrimSpace(id)
	for _, user := range users {
		if user.ID == id {
			return user, true
		}
	}
	return im.User{}, false
}

func (s *Service) CreateRoom(req im.CreateRoomRequest) (im.Room, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return im.Room{}, fmt.Errorf("title is required")
	}
	creatorID := strings.TrimSpace(req.CreatorID)
	if creatorID == "" {
		return im.Room{}, fmt.Errorf("creator_id is required")
	}

	app, err := s.appConfigForCreator(creatorID)
	if err != nil {
		return im.Room{}, err
	}
	adminOpenID := strings.TrimSpace(s.defaultAdminOpenID(app))
	if adminOpenID == "" {
		return im.Room{}, fmt.Errorf("feishu admin_open_id is required")
	}
	members := normalizeMembers(creatorID, req.MemberIDs)
	memberBotIDs := members[1:]
	memberAppIDs, err := s.appIDsForMembers(memberBotIDs)
	if err != nil {
		return im.Room{}, err
	}
	description := strings.TrimSpace(req.Description)

	created, err := s.createChat(context.Background(), app, CreateChatRequest{
		Title:       title,
		Description: description,
		CreatorID:   adminOpenID,
	})
	if err != nil {
		return im.Room{}, err
	}
	chatID := strings.TrimSpace(created.ChatID)
	if chatID == "" {
		return im.Room{}, fmt.Errorf("create feishu chat: empty chat_id in response")
	}
	if responseName := strings.TrimSpace(created.Name); responseName != "" {
		title = responseName
	}
	if responseDescription := strings.TrimSpace(created.Description); responseDescription != "" {
		description = responseDescription
	}
	if len(memberAppIDs) > 0 {
		if err := s.addChatMembers(context.Background(), app, AddChatMembersRequest{
			ChatID:       chatID,
			MemberBotIDs: memberBotIDs,
			MemberAppIDs: memberAppIDs,
		}); err != nil {
			return im.Room{}, fmt.Errorf("create feishu chat %s succeeded but add members failed: %w", chatID, err)
		}
	}

	room := im.Room{
		ID:          chatID,
		Title:       title,
		Subtitle:    formatMembers(len(members)),
		Description: description,
		Members:     members,
		Messages:    nil,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.rooms[room.ID] = &room
	return cloneRoom(room), nil
}

func defaultCreateChat(ctx context.Context, app AppConfig, req CreateChatRequest) (CreateChatResponse, error) {
	client := lark.NewClient(app.AppID, app.AppSecret)
	createReq := larkim.NewCreateChatReqBuilder().
		UserIdType("open_id"). // OwnerId is an open_id; BotIdList below expects bot app_id values.
		SetBotManager(true).
		Uuid(feishuRequestUUID()).
		Body(larkim.NewCreateChatReqBodyBuilder().
			Name(req.Title).
			Description(req.Description).
			OwnerId(req.CreatorID).
			UserIdList([]string{}).
			BotIdList(req.MemberAppIDs).
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

	resp, err := client.Im.V1.Chat.Create(ctx, createReq)
	if err != nil {
		return CreateChatResponse{}, fmt.Errorf("create feishu chat: %w", err)
	}
	if !resp.Success() {
		return CreateChatResponse{}, fmt.Errorf(
			"create feishu chat: code=%d msg=%s request_id=%s request_app_id=%s owner_open_id=%s bot_app_ids=%s response_data=%s response_body=%s",
			resp.Code,
			resp.Msg,
			resp.RequestId(),
			strings.TrimSpace(app.AppID),
			req.CreatorID,
			strings.Join(normalizeNonEmptyStrings(req.MemberAppIDs), ","),
			feishuResponseData(resp.Data),
			feishuResponseBody(resp.RawBody),
		)
	}
	if resp.Data == nil {
		return CreateChatResponse{}, fmt.Errorf("create feishu chat: empty response data")
	}
	return CreateChatResponse{
		ChatID:      larkcore.StringValue(resp.Data.ChatId),
		Name:        larkcore.StringValue(resp.Data.Name),
		Description: larkcore.StringValue(resp.Data.Description),
	}, nil
}

func defaultAddChatMembers(ctx context.Context, app AppConfig, req AddChatMembersRequest) error {
	memberAppIDs := normalizeNonEmptyStrings(req.MemberAppIDs)
	if len(memberAppIDs) == 0 {
		return fmt.Errorf("add feishu chat members: member app_ids are required")
	}

	client := lark.NewClient(app.AppID, app.AppSecret)
	addReq := larkim.NewCreateChatMembersReqBuilder().
		ChatId(req.ChatID).
		MemberIdType("app_id").
		SucceedType(2).
		Body(larkim.NewCreateChatMembersReqBodyBuilder().
			IdList(memberAppIDs).
			Build()).
		Build()

	resp, err := client.Im.V1.ChatMembers.Create(ctx, addReq)
	if err != nil {
		return fmt.Errorf("add feishu chat members: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf(
			"add feishu chat members: code=%d msg=%s request_id=%s request_app_id=%s chat_id=%s member_id_type=app_id succeed_type=2 member_app_ids=%s response_data=%s response_body=%s",
			resp.Code,
			resp.Msg,
			resp.RequestId(),
			strings.TrimSpace(app.AppID),
			strings.TrimSpace(req.ChatID),
			strings.Join(memberAppIDs, ","),
			feishuResponseData(resp.Data),
			feishuResponseBody(resp.RawBody),
		)
	}
	return nil
}

func feishuResponseData(data any) string {
	if data == nil {
		return "null"
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf("%T", data)
	}
	return string(raw)
}

func feishuResponseBody(raw []byte) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "null"
	}
	var value any
	if err := json.Unmarshal(raw, &value); err == nil {
		if normalized, err := json.Marshal(value); err == nil {
			return string(normalized)
		}
	}
	return string(raw)
}

func defaultDeleteChat(ctx context.Context, app AppConfig, chatID string) error {
	client := lark.NewClient(app.AppID, app.AppSecret)
	resp, err := client.Im.V1.Chat.Delete(ctx, larkim.NewDeleteChatReqBuilder().
		ChatId(chatID).
		Build())
	if err != nil {
		return fmt.Errorf("delete feishu chat: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("delete feishu chat: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	return nil
}

func defaultListChatMembers(ctx context.Context, app AppConfig, apps map[string]AppConfig, chatID string) ([]im.User, error) {
	client := lark.NewClient(app.AppID, app.AppSecret)
	members := make([]im.User, 0)
	memberIDs := make(map[string]struct{})
	pageToken := ""

	for {
		reqBuilder := larkim.NewGetChatMembersReqBuilder().
			ChatId(chatID).
			MemberIdType("open_id").
			PageSize(100)
		if pageToken != "" {
			reqBuilder.PageToken(pageToken)
		}

		resp, err := client.Im.V1.ChatMembers.Get(ctx, reqBuilder.Build())
		if err != nil {
			return nil, fmt.Errorf("list feishu chat members: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("list feishu chat members: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
		}
		if resp.Data == nil {
			return nil, fmt.Errorf("list feishu chat members: empty response data")
		}

		for _, item := range resp.Data.Items {
			if item == nil {
				continue
			}
			memberID := strings.TrimSpace(larkcore.StringValue(item.MemberId))
			if memberID == "" {
				continue
			}
			if _, ok := memberIDs[memberID]; ok {
				continue
			}
			name := strings.TrimSpace(larkcore.StringValue(item.Name))
			if name == "" {
				name = memberID
			}
			memberIDs[memberID] = struct{}{}
			members = append(members, im.User{
				ID:        memberID,
				Name:      name,
				Role:      "member",
				Avatar:    initials(name),
				IsOnline:  true,
				AccentHex: accentHexForID(memberID),
			})
		}

		if !larkcore.BoolValue(resp.Data.HasMore) {
			break
		}
		pageToken = strings.TrimSpace(larkcore.StringValue(resp.Data.PageToken))
		if pageToken == "" {
			break
		}
	}

	botMembers, err := feishuBotMembersInChat(ctx, apps, chatID, memberIDs)
	if err != nil {
		return nil, err
	}
	members = append(members, botMembers...)

	return members, nil
}

func feishuBotMembersInChat(ctx context.Context, apps map[string]AppConfig, chatID string, existingMemberIDs map[string]struct{}) ([]im.User, error) {
	return feishuBotMembersInChatWithResolvers(ctx, apps, chatID, existingMemberIDs, fetchBotInfo, feishuAppIsInChat)
}

func feishuBotMembersInChatWithResolvers(
	ctx context.Context,
	apps map[string]AppConfig,
	chatID string,
	existingMemberIDs map[string]struct{},
	resolveBotInfo func(context.Context, AppConfig) (BotInfo, error),
	isInChat func(context.Context, AppConfig, string) (bool, error),
) ([]im.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if existingMemberIDs == nil {
		existingMemberIDs = make(map[string]struct{})
	}

	members := make([]im.User, 0, len(apps))
	for botID, rawApp := range apps {
		app, err := validateAppConfig(rawApp, botID)
		if err != nil {
			return nil, err
		}

		inChat, err := isInChat(ctx, app, chatID)
		if err != nil {
			return nil, fmt.Errorf("check feishu bot %q in chat %q: %w", botID, chatID, err)
		}
		if !inChat {
			continue
		}

		botInfo, err := resolveBotInfo(ctx, app)
		if err != nil {
			return nil, fmt.Errorf("resolve feishu bot %q open_id: %w", botID, err)
		}
		openID := strings.TrimSpace(botInfo.OpenID)
		if openID == "" {
			return nil, fmt.Errorf("resolve feishu bot %q open_id: empty open_id", botID)
		}
		if _, ok := existingMemberIDs[openID]; ok {
			continue
		}
		existingMemberIDs[openID] = struct{}{}

		name := strings.TrimSpace(botID)
		if name == "" {
			name = openID
		}
		members = append(members, im.User{
			ID:        botID,
			Name:      name,
			Role:      "member",
			Avatar:    initials(name),
			IsOnline:  true,
			AccentHex: accentHexForID(botID),
		})
	}
	return members, nil
}

func feishuAppIsInChat(ctx context.Context, app AppConfig, chatID string) (bool, error) {
	client := lark.NewClient(app.AppID, app.AppSecret)
	resp, err := client.Im.V1.ChatMembers.IsInChat(ctx, larkim.NewIsInChatChatMembersReqBuilder().
		ChatId(chatID).
		Build())
	if err != nil {
		return false, err
	}
	if !resp.Success() {
		return false, fmt.Errorf("code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	if resp.Data == nil {
		return false, fmt.Errorf("empty response data")
	}
	return larkcore.BoolValue(resp.Data.IsInChat), nil
}

func defaultListChats(ctx context.Context, app AppConfig) ([]im.Room, error) {
	client := lark.NewClient(app.AppID, app.AppSecret)
	rooms := make([]im.Room, 0)
	pageToken := ""

	for {
		reqBuilder := larkim.NewListChatReqBuilder().
			UserIdType("open_id").
			SortType("ByCreateTimeAsc").
			PageSize(100)
		if pageToken != "" {
			reqBuilder.PageToken(pageToken)
		}

		resp, err := client.Im.V1.Chat.List(ctx, reqBuilder.Build())
		if err != nil {
			return nil, fmt.Errorf("list feishu chats: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("list feishu chats: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
		}
		if resp.Data == nil {
			return nil, fmt.Errorf("list feishu chats: empty response data")
		}

		for _, item := range resp.Data.Items {
			if item == nil {
				continue
			}
			chatID := strings.TrimSpace(larkcore.StringValue(item.ChatId))
			if chatID == "" {
				continue
			}
			title := strings.TrimSpace(larkcore.StringValue(item.Name))
			if title == "" {
				title = chatID
			}
			description := strings.TrimSpace(larkcore.StringValue(item.Description))
			members := normalizeNonEmptyStrings([]string{larkcore.StringValue(item.OwnerId)})
			rooms = append(rooms, im.Room{
				ID:          chatID,
				Title:       title,
				Subtitle:    formatMembers(len(members)),
				Description: description,
				Members:     members,
				Messages:    nil,
			})
		}

		if !larkcore.BoolValue(resp.Data.HasMore) {
			break
		}
		pageToken = strings.TrimSpace(larkcore.StringValue(resp.Data.PageToken))
		if pageToken == "" {
			break
		}
	}

	return rooms, nil
}

func defaultListRoomMessages(ctx context.Context, app AppConfig, chatID string) ([]im.Message, error) {
	client := lark.NewClient(app.AppID, app.AppSecret)
	messages := make([]im.Message, 0)
	pageToken := ""

	for {
		reqBuilder := larkim.NewListMessageReqBuilder().
			ContainerIdType("chat").
			ContainerId(chatID).
			StartTime("0").
			EndTime(fmt.Sprint(time.Now().UTC().Unix())).
			SortType("ByCreateTimeAsc").
			PageSize(50)
		if pageToken != "" {
			reqBuilder.PageToken(pageToken)
		}

		resp, err := client.Im.V1.Message.List(ctx, reqBuilder.Build())
		if err != nil {
			return nil, fmt.Errorf("list feishu messages: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("list feishu messages: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
		}
		if resp.Data == nil {
			return nil, fmt.Errorf("list feishu messages: empty response data")
		}

		for _, item := range resp.Data.Items {
			message, ok := feishuSDKMessageToIMMessage(item)
			if ok {
				messages = append(messages, message)
			}
		}

		if !larkcore.BoolValue(resp.Data.HasMore) {
			break
		}
		pageToken = strings.TrimSpace(larkcore.StringValue(resp.Data.PageToken))
		if pageToken == "" {
			break
		}
	}

	return messages, nil
}

func feishuSDKMessageToIMMessage(item *larkim.Message) (im.Message, bool) {
	if item == nil || larkcore.BoolValue(item.Deleted) {
		return im.Message{}, false
	}

	messageID := strings.TrimSpace(larkcore.StringValue(item.MessageId))
	if messageID == "" {
		return im.Message{}, false
	}
	senderID := ""
	if item.Sender != nil {
		senderID = strings.TrimSpace(larkcore.StringValue(item.Sender.Id))
	}
	content := ""
	if item.Body != nil {
		content = feishuMessageContentText(larkcore.StringValue(item.Body.Content))
	}

	return im.Message{
		ID:        messageID,
		SenderID:  senderID,
		Kind:      im.MessageKindMessage,
		Content:   content,
		CreatedAt: feishuMessageCreatedAt(larkcore.StringValue(item.CreateTime)),
		Mentions:  feishuMessageMentions(item.Mentions),
	}, true
}

func feishuMessageContentText(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	var textContent struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &textContent); err == nil && textContent.Text != "" {
		return textContent.Text
	}
	return content
}

func feishuMessageCreatedAt(createTime string) time.Time {
	createTime = strings.TrimSpace(createTime)
	if createTime == "" {
		return time.Time{}
	}
	timestamp, err := time.ParseDuration(createTime + "ms")
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(timestamp.Milliseconds()).UTC()
}

func feishuMessageMentions(mentions []*larkim.Mention) []im.Mention {
	result := make([]im.Mention, 0, len(mentions))
	for _, mention := range mentions {
		if mention == nil {
			continue
		}
		id := larkcore.StringValue(mention.Id)
		if strings.TrimSpace(id) == "" {
			continue
		}
		result = append(result, im.Mention{
			ID:   id,
			Name: larkcore.StringValue(mention.Name),
		})
	}
	return result
}

func defaultSendMessage(ctx context.Context, app AppConfig, req SendMessageRequest) (SendMessageResponse, error) {
	senderInfo, err := fetchBotInfo(ctx, app)
	if err != nil {
		return SendMessageResponse{}, err
	}
	senderOpenID := senderInfo.OpenID
	mentionID := strings.TrimSpace(req.MentionID)
	mentionOpenID := ""
	if mentionID != "" {
		mentionOpenID = strings.TrimSpace(req.MentionOpenID)
		if mentionOpenID == "" {
			mentionApp, err := validateAppConfig(req.MentionAppConfig, mentionID)
			if err != nil {
				return SendMessageResponse{}, err
			}
			botInfo, err := fetchBotInfo(ctx, mentionApp)
			if err != nil {
				return SendMessageResponse{}, err
			}
			mentionOpenID = botInfo.OpenID
		}
	}

	content, err := feishuTextMessageContent(req.Content, mentionID, mentionOpenID)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("encode feishu message content: %w", err)
	}

	client := lark.NewClient(app.AppID, app.AppSecret)
	if threadRootID := strings.TrimSpace(req.ThreadRootID); threadRootID != "" {
		replyReq := larkim.NewReplyMessageReqBuilder().
			MessageId(threadRootID).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				MsgType("text").
				Content(string(content)).
				ReplyInThread(true).
				Uuid(req.UUID).
				Build()).
			Build()
		resp, err := client.Im.V1.Message.Reply(ctx, replyReq)
		if err != nil {
			return SendMessageResponse{}, fmt.Errorf("reply feishu message: %w", err)
		}
		if !resp.Success() {
			return SendMessageResponse{}, fmt.Errorf("reply feishu message: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
		}
		if resp.Data == nil {
			return SendMessageResponse{}, fmt.Errorf("reply feishu message: empty response data")
		}
		return SendMessageResponse{
			MessageID:     larkcore.StringValue(resp.Data.MessageId),
			SenderOpenID:  senderOpenID,
			MentionOpenID: mentionOpenID,
		}, nil
	}

	sendReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(req.ChatID).
			MsgType("text").
			Content(string(content)).
			Uuid(req.UUID).
			Build()).
		Build()

	resp, err := client.Im.V1.Message.Create(ctx, sendReq)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("send feishu message: %w", err)
	}
	if !resp.Success() {
		return SendMessageResponse{}, fmt.Errorf("send feishu message: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	if resp.Data == nil {
		return SendMessageResponse{}, fmt.Errorf("send feishu message: empty response data")
	}
	return SendMessageResponse{
		MessageID:     larkcore.StringValue(resp.Data.MessageId),
		SenderOpenID:  senderOpenID,
		MentionOpenID: mentionOpenID,
	}, nil
}

func defaultUpdateMessage(ctx context.Context, app AppConfig, req UpdateMessageRequest) (UpdateMessageResponse, error) {
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		return UpdateMessageResponse{}, fmt.Errorf("message_id is required")
	}
	content, err := feishuTextMessageContent(req.Content, "", "")
	if err != nil {
		return UpdateMessageResponse{}, fmt.Errorf("encode feishu message content: %w", err)
	}

	client := lark.NewClient(app.AppID, app.AppSecret)
	updateReq := larkim.NewUpdateMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewUpdateMessageReqBodyBuilder().
			MsgType("text").
			Content(content).
			Build()).
		Build()
	resp, err := client.Im.V1.Message.Update(ctx, updateReq)
	if err != nil {
		return UpdateMessageResponse{}, fmt.Errorf("update feishu message: %w", err)
	}
	if !resp.Success() {
		return UpdateMessageResponse{}, fmt.Errorf("update feishu message: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	if resp.Data == nil {
		return UpdateMessageResponse{}, fmt.Errorf("update feishu message: empty response data")
	}
	updatedMessageID := strings.TrimSpace(larkcore.StringValue(resp.Data.MessageId))
	if updatedMessageID == "" {
		updatedMessageID = messageID
	}
	return UpdateMessageResponse{MessageID: updatedMessageID}, nil
}

func defaultCreateMessageReaction(ctx context.Context, app AppConfig, req CreateMessageReactionRequest) (CreateMessageReactionResponse, error) {
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		return CreateMessageReactionResponse{}, fmt.Errorf("message_id is required")
	}
	emojiType := strings.TrimSpace(req.EmojiType)
	if emojiType == "" {
		return CreateMessageReactionResponse{}, fmt.Errorf("emoji_type is required")
	}

	client := lark.NewClient(app.AppID, app.AppSecret)
	createReq := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(messageID).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(larkim.NewEmojiBuilder().EmojiType(emojiType).Build()).
			Build()).
		Build()
	resp, err := client.Im.V1.MessageReaction.Create(ctx, createReq)
	if err != nil {
		return CreateMessageReactionResponse{}, fmt.Errorf("create feishu message reaction: %w", err)
	}
	if !resp.Success() {
		return CreateMessageReactionResponse{}, fmt.Errorf("create feishu message reaction: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	if resp.Data == nil {
		return CreateMessageReactionResponse{}, fmt.Errorf("create feishu message reaction: empty response data")
	}
	reactionID := strings.TrimSpace(larkcore.StringValue(resp.Data.ReactionId))
	if reactionID == "" {
		return CreateMessageReactionResponse{}, fmt.Errorf("create feishu message reaction: empty reaction id")
	}
	return CreateMessageReactionResponse{ReactionID: reactionID}, nil
}

func defaultDeleteMessageReaction(ctx context.Context, app AppConfig, req DeleteMessageReactionRequest) error {
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		return fmt.Errorf("message_id is required")
	}
	reactionID := strings.TrimSpace(req.ReactionID)
	if reactionID == "" {
		return fmt.Errorf("reaction_id is required")
	}

	client := lark.NewClient(app.AppID, app.AppSecret)
	deleteReq := larkim.NewDeleteMessageReactionReqBuilder().
		MessageId(messageID).
		ReactionId(reactionID).
		Build()
	resp, err := client.Im.V1.MessageReaction.Delete(ctx, deleteReq)
	if err != nil {
		return fmt.Errorf("delete feishu message reaction: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("delete feishu message reaction: code=%d msg=%s request_id=%s", resp.Code, resp.Msg, resp.RequestId())
	}
	return nil
}

func feishuTextMessageContent(content, mentionID, mentionOpenID string) (string, error) {
	text := slashcommand.RenderFeishuFallback(content)
	mentionID = strings.TrimSpace(mentionID)
	mentionOpenID = strings.TrimSpace(mentionOpenID)
	if mentionID != "" && mentionOpenID != "" {
		text = fmt.Sprintf("<at user_id=\"%s\">%s</at> %s", mentionOpenID, mentionID, text)
	}
	data, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// fetchBotInfo calls the Feishu bot info API to retrieve bot identity fields.
func fetchBotInfo(ctx context.Context, app AppConfig) (BotInfo, error) {
	client := lark.NewClient(app.AppID, app.AppSecret)
	resp, err := client.Do(ctx, &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   "/open-apis/bot/v3/info",
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
	})
	if err != nil {
		return BotInfo{}, fmt.Errorf("bot info request: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"bot"`
	}
	if err := json.Unmarshal(resp.RawBody, &result); err != nil {
		return BotInfo{}, fmt.Errorf("bot info parse: %w", err)
	}
	if result.Code != 0 {
		return BotInfo{}, fmt.Errorf("bot info api error: code=%d msg=%s", result.Code, result.Msg)
	}
	if result.Bot.OpenID == "" {
		return BotInfo{}, fmt.Errorf("bot info: empty open_id")
	}
	return BotInfo{
		OpenID:  result.Bot.OpenID,
		AppName: result.Bot.AppName,
	}, nil
}

func (s *Service) SendMessage(req im.CreateMessageRequest) (im.Message, error) {
	roomID := strings.TrimSpace(req.RoomID)
	senderID := strings.TrimSpace(req.SenderID)
	content := strings.TrimSpace(req.Content)
	if roomID == "" {
		return im.Message{}, fmt.Errorf("room_id is required")
	}
	if senderID == "" {
		return im.Message{}, fmt.Errorf("sender_id is required")
	}
	if content == "" {
		return im.Message{}, fmt.Errorf("content is required")
	}
	var relatesTo *im.MessageRelation
	var threadRootID string
	if req.RelatesTo != nil {
		relType := strings.TrimSpace(req.RelatesTo.RelType)
		eventID := strings.TrimSpace(req.RelatesTo.EventID)
		if relType != "" || eventID != "" {
			if relType != im.RelationTypeThread {
				return im.Message{}, fmt.Errorf("unsupported relation type")
			}
			if eventID == "" {
				return im.Message{}, fmt.Errorf("relation event_id is required")
			}
			relatesTo = &im.MessageRelation{RelType: im.RelationTypeThread, EventID: eventID}
			threadRootID = eventID
		}
	}

	s.mu.RLock()
	app, err := s.appConfigForSenderLocked(senderID)
	mentionID := strings.TrimSpace(req.MentionID)
	var mentionApp AppConfig
	var mentionOpenID string
	if err == nil && mentionID != "" {
		if openID, ok := s.participantMentionOpenIDLocked(mentionID); ok {
			mentionOpenID = openID
		} else {
			mentionApp, err = s.appConfigForMentionLocked(mentionID)
		}
	}
	s.mu.RUnlock()
	if err != nil {
		return im.Message{}, err
	}

	fallbackID := fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano())
	sent, err := s.sendMessage(context.Background(), app, SendMessageRequest{
		ChatID:           roomID,
		Content:          content,
		UUID:             fallbackID,
		ThreadRootID:     threadRootID,
		MentionID:        mentionID,
		MentionOpenID:    mentionOpenID,
		MentionAppConfig: mentionApp,
	})
	if err != nil {
		return im.Message{}, err
	}
	senderOpenID := strings.TrimSpace(sent.SenderOpenID)
	if senderOpenID == "" {
		return im.Message{}, fmt.Errorf("resolve feishu sender open_id: empty open_id for %q", senderID)
	}
	sentMentionOpenID := strings.TrimSpace(sent.MentionOpenID)
	if sentMentionOpenID == "" {
		sentMentionOpenID = mentionOpenID
	}
	if mentionID != "" && sentMentionOpenID == "" {
		return im.Message{}, fmt.Errorf("resolve feishu mention open_id: empty open_id for %q", mentionID)
	}

	sentMessageID := strings.TrimSpace(sent.MessageID)
	if sentMessageID == "" {
		sentMessageID = fallbackID
	}
	message := im.Message{
		ID:        sentMessageID,
		SenderID:  senderOpenID,
		Kind:      im.MessageKindMessage,
		Content:   content,
		Metadata:  utils.CloneAnyMap(req.Metadata),
		CreatedAt: time.Now().UTC(),
		Mentions:  nil,
		RelatesTo: relatesTo,
	}
	if sentMentionOpenID != "" {
		message.Mentions = []im.Mention{{ID: sentMentionOpenID, Name: mentionID}}
	}

	s.mu.Lock()
	if room, ok := s.rooms[roomID]; ok {
		room.Messages = append(room.Messages, message)
	}
	s.mu.Unlock()

	if len(message.Mentions) > 0 {
		s.messageBus.Publish(MessageEvent{
			Type:         MessageEventTypeMessageCreated,
			RoomID:       roomID,
			SenderBotID:  senderID,
			MentionBotID: mentionID,
			Message:      &message,
		})
	}
	return message, nil
}

func (s *Service) UpdateMessage(req UpdateMessageRequest) (im.Message, error) {
	return s.UpdateMessageWithContext(context.Background(), req)
}

func (s *Service) UpdateMessageWithContext(ctx context.Context, req UpdateMessageRequest) (im.Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	roomID := strings.TrimSpace(req.RoomID)
	senderID := strings.TrimSpace(req.SenderID)
	messageID := strings.TrimSpace(req.MessageID)
	content := strings.TrimSpace(req.Content)
	if roomID == "" {
		return im.Message{}, fmt.Errorf("room_id is required")
	}
	if senderID == "" {
		return im.Message{}, fmt.Errorf("sender_id is required")
	}
	if messageID == "" {
		return im.Message{}, fmt.Errorf("message_id is required")
	}
	if content == "" {
		return im.Message{}, fmt.Errorf("content is required")
	}

	s.mu.RLock()
	app, err := s.appConfigForSenderLocked(senderID)
	s.mu.RUnlock()
	if err != nil {
		return im.Message{}, err
	}

	updated, err := s.updateMessage(ctx, app, UpdateMessageRequest{
		RoomID:    roomID,
		SenderID:  senderID,
		MessageID: messageID,
		Content:   content,
	})
	if err != nil {
		return im.Message{}, err
	}
	updatedMessageID := strings.TrimSpace(updated.MessageID)
	if updatedMessageID == "" {
		updatedMessageID = messageID
	}

	message := im.Message{
		ID:        updatedMessageID,
		Kind:      im.MessageKindMessage,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	if room, ok := s.rooms[roomID]; ok {
		for idx := range room.Messages {
			if strings.TrimSpace(room.Messages[idx].ID) != messageID {
				continue
			}
			message = room.Messages[idx]
			message.ID = updatedMessageID
			message.Content = content
			room.Messages[idx] = message
			break
		}
	}
	s.mu.Unlock()
	return message, nil
}

func (s *Service) CreateMessageReaction(req CreateMessageReactionRequest) (CreateMessageReactionResponse, error) {
	return s.CreateMessageReactionWithContext(context.Background(), req)
}

func (s *Service) CreateMessageReactionWithContext(ctx context.Context, req CreateMessageReactionRequest) (CreateMessageReactionResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	senderID := strings.TrimSpace(req.SenderID)
	messageID := strings.TrimSpace(req.MessageID)
	emojiType := strings.TrimSpace(req.EmojiType)
	if senderID == "" {
		return CreateMessageReactionResponse{}, fmt.Errorf("sender_id is required")
	}
	if messageID == "" {
		return CreateMessageReactionResponse{}, fmt.Errorf("message_id is required")
	}
	if emojiType == "" {
		return CreateMessageReactionResponse{}, fmt.Errorf("emoji_type is required")
	}

	s.mu.RLock()
	app, err := s.appConfigForSenderLocked(senderID)
	s.mu.RUnlock()
	if err != nil {
		return CreateMessageReactionResponse{}, err
	}

	return s.createMessageReaction(ctx, app, CreateMessageReactionRequest{
		SenderID:  senderID,
		MessageID: messageID,
		EmojiType: emojiType,
	})
}

func (s *Service) DeleteMessageReaction(req DeleteMessageReactionRequest) error {
	return s.DeleteMessageReactionWithContext(context.Background(), req)
}

func (s *Service) DeleteMessageReactionWithContext(ctx context.Context, req DeleteMessageReactionRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	senderID := strings.TrimSpace(req.SenderID)
	messageID := strings.TrimSpace(req.MessageID)
	reactionID := strings.TrimSpace(req.ReactionID)
	if senderID == "" {
		return fmt.Errorf("sender_id is required")
	}
	if messageID == "" {
		return fmt.Errorf("message_id is required")
	}
	if reactionID == "" {
		return fmt.Errorf("reaction_id is required")
	}

	s.mu.RLock()
	app, err := s.appConfigForSenderLocked(senderID)
	s.mu.RUnlock()
	if err != nil {
		return err
	}

	return s.deleteMessageReaction(ctx, app, DeleteMessageReactionRequest{
		SenderID:   senderID,
		MessageID:  messageID,
		ReactionID: reactionID,
	})
}

func (s *Service) ResolveBotOpenID(ctx context.Context, botID string) (string, string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return "", "", fmt.Errorf("feishu bot id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.RLock()
	app, ok := s.appConfigByIDLocked(botID)
	s.mu.RUnlock()
	if !ok {
		return botID, "", nil
	}

	app, err := validateAppConfig(app, botID)
	if err != nil {
		return "", "", err
	}
	botInfo, err := s.resolveBotInfo(ctx, app)
	if err != nil {
		return "", "", err
	}
	return botInfo.OpenID, botInfo.AppName, nil
}

func (s *Service) ListRooms() ([]im.Room, error) {
	app, err := s.appConfigForRoom("")
	if err != nil {
		return nil, err
	}

	rooms, err := s.listChats(context.Background(), app)
	if err != nil {
		return nil, err
	}
	identity, err := s.botIdentityMap(context.Background())
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range rooms {
		rooms[i].Members = identity.memberIDsToBotIDs(rooms[i].Members)
		if local, ok := s.rooms[rooms[i].ID]; ok {
			rooms[i].Messages = identity.messagesToBotMessages(local.Messages)
		}
		rooms[i].Subtitle = formatMembers(len(rooms[i].Members))
	}
	slices.SortFunc(rooms, func(a, b im.Room) int { return strings.Compare(a.Title, b.Title) })
	return rooms, nil
}

func (s *Service) ListRoomMessages(roomID string) ([]im.Message, error) {
	app, err := s.appConfigForRoom("")
	if err != nil {
		return nil, err
	}

	messages, err := s.listRoomMessages(context.Background(), app, roomID)
	if err != nil {
		return nil, err
	}
	identity, err := s.botIdentityMap(context.Background())
	if err != nil {
		return nil, err
	}
	messages = identity.messagesToBotMessages(messages)

	s.mu.Lock()
	if room, ok := s.rooms[roomID]; ok {
		room.Messages = append([]im.Message(nil), messages...)
	}
	s.mu.Unlock()

	return append([]im.Message(nil), messages...), nil
}

func (s *Service) DeleteRoom(roomID string) error {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return fmt.Errorf("room_id is required")
	}

	app, err := s.appConfigForRoom(roomID)
	if err != nil {
		return err
	}
	if err := s.deleteChat(context.Background(), app, roomID); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.rooms, roomID)
	s.mu.Unlock()
	return nil
}

func (s *Service) AddRoomMembers(req im.AddRoomMembersRequest) (im.Room, error) {
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return im.Room{}, fmt.Errorf("room_id is required")
	}
	if len(req.UserIDs) == 0 {
		return im.Room{}, fmt.Errorf("user_ids is required")
	}

	s.mu.Lock()
	room, ok := s.rooms[roomID]
	existing := make(map[string]struct{})
	if ok {
		for _, userID := range room.Members {
			existing[userID] = struct{}{}
		}
	}

	newMembers := make([]string, 0, len(req.UserIDs))
	newMemberAppIDs := make([]string, 0, len(req.UserIDs))
	for _, userID := range req.UserIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := existing[userID]; ok {
			continue
		}
		existing[userID] = struct{}{}
		newMembers = append(newMembers, userID)
		memberAppID, err := s.appIDForMemberLocked(userID)
		if err != nil {
			s.mu.Unlock()
			return im.Room{}, err
		}
		newMemberAppIDs = append(newMemberAppIDs, memberAppID)
	}
	if len(newMembers) == 0 {
		s.mu.Unlock()
		return im.Room{}, fmt.Errorf("no new users to invite")
	}
	appOwnerID := strings.TrimSpace(req.InviterID)
	if appOwnerID == "" && room != nil && len(room.Members) > 0 {
		appOwnerID = room.Members[0]
	}
	app, err := s.appConfigForCreatorLocked(appOwnerID)
	if err != nil {
		s.mu.Unlock()
		return im.Room{}, err
	}
	s.mu.Unlock()

	if err := s.addChatMembers(context.Background(), app, AddChatMembersRequest{
		ChatID:       roomID,
		MemberBotIDs: newMembers,
		MemberAppIDs: newMemberAppIDs,
	}); err != nil {
		return im.Room{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok = s.rooms[roomID]
	if !ok {
		return im.Room{
			ID:       roomID,
			Subtitle: formatMembers(len(newMembers)),
			Members:  append([]string(nil), newMembers...),
		}, nil
	}
	existing = make(map[string]struct{}, len(room.Members))
	for _, userID := range room.Members {
		existing[userID] = struct{}{}
	}
	for _, userID := range newMembers {
		if _, ok := existing[userID]; ok {
			continue
		}
		room.Members = append(room.Members, userID)
	}
	room.Subtitle = formatMembers(len(room.Members))
	return cloneRoom(*room), nil
}

func (s *Service) ListRoomMembers(roomID string) ([]im.User, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return nil, fmt.Errorf("room_id is required")
	}

	app, err := s.appConfigForRoom(roomID)
	if err != nil {
		return nil, err
	}

	members, err := s.listChatMembers(context.Background(), app, s.AppConfigs(), roomID)
	if err != nil {
		return nil, err
	}
	identity, err := s.botIdentityMap(context.Background())
	if err != nil {
		return nil, err
	}
	members = identity.usersToBotUsers(members)

	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]im.User, 0, len(members))
	for _, member := range members {
		if localUser, ok := s.users[member.ID]; ok {
			if member.Name != "" {
				localUser.Name = member.Name
			}
			users = append(users, localUser)
			continue
		}
		users = append(users, member)
	}
	slices.SortFunc(users, func(a, b im.User) int { return strings.Compare(a.Name, b.Name) })
	return users, nil
}

type botIdentityMap struct {
	botIDs        map[string]struct{}
	openIDToBotID map[string]string
}

func (s *Service) botIdentityMap(ctx context.Context) (botIdentityMap, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	apps := s.AppConfigs()
	s.mu.RLock()
	resolveBotInfo := s.resolveBotInfo
	s.mu.RUnlock()

	identity := botIdentityMap{
		botIDs:        make(map[string]struct{}, len(apps)),
		openIDToBotID: make(map[string]string, len(apps)),
	}
	for botID, rawApp := range apps {
		botID = strings.TrimSpace(botID)
		if botID == "" {
			continue
		}
		identity.botIDs[botID] = struct{}{}
		app, err := validateAppConfig(rawApp, botID)
		if err != nil {
			return identity, err
		}
		botInfo, err := resolveBotInfo(ctx, app)
		if err != nil {
			return identity, fmt.Errorf("resolve feishu bot %q open_id: %w", botID, err)
		}
		openID := strings.TrimSpace(botInfo.OpenID)
		if openID == "" {
			return identity, fmt.Errorf("resolve feishu bot %q open_id: empty open_id", botID)
		}
		identity.openIDToBotID[openID] = botID
	}
	return identity, nil
}

func (m botIdentityMap) botIDFor(id string) (string, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false
	}
	if botID, ok := m.openIDToBotID[id]; ok {
		return botID, true
	}
	if _, ok := m.botIDs[id]; ok {
		return id, true
	}
	return "", false
}

func (m botIdentityMap) memberIDsToBotIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if botID, ok := m.botIDFor(id); ok {
			id = botID
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (m botIdentityMap) usersToBotUsers(members []im.User) []im.User {
	if len(members) == 0 {
		return nil
	}
	out := make([]im.User, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		member.ID = strings.TrimSpace(member.ID)
		if member.ID == "" {
			continue
		}
		if botID, ok := m.botIDFor(member.ID); ok {
			member.ID = botID
			if strings.TrimSpace(member.Name) == "" {
				member.Name = botID
			}
			if strings.TrimSpace(member.Avatar) == "" {
				member.Avatar = initials(member.Name)
			}
			if strings.TrimSpace(member.AccentHex) == "" {
				member.AccentHex = accentHexForID(botID)
			}
		}
		if _, ok := seen[member.ID]; ok {
			continue
		}
		seen[member.ID] = struct{}{}
		out = append(out, member)
	}
	return out
}

func (m botIdentityMap) messagesToBotMessages(messages []im.Message) []im.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]im.Message, 0, len(messages))
	for _, message := range messages {
		if senderID := strings.TrimSpace(message.SenderID); senderID != "" {
			if botID, ok := m.botIDFor(senderID); ok {
				message.SenderID = botID
			}
		}
		message.Mentions = m.mentionsToBotMentions(message.Mentions)
		out = append(out, message)
	}
	return out
}

func (m botIdentityMap) mentionsToBotMentions(mentions []im.Mention) []im.Mention {
	if len(mentions) == 0 {
		return nil
	}
	out := make([]im.Mention, 0, len(mentions))
	seen := make(map[string]struct{}, len(mentions))
	for _, mention := range mentions {
		mention.ID = strings.TrimSpace(mention.ID)
		if mention.ID == "" {
			continue
		}
		if botID, ok := m.botIDFor(mention.ID); ok {
			mention.ID = botID
			if strings.TrimSpace(mention.Name) == "" {
				mention.Name = botID
			}
		}
		if _, ok := seen[mention.ID]; ok {
			continue
		}
		seen[mention.ID] = struct{}{}
		out = append(out, mention)
	}
	return out
}

func (s *Service) normalizeMembersLocked(creatorID string, memberIDs []string) ([]string, error) {
	seen := map[string]struct{}{creatorID: {}}
	members := []string{creatorID}
	for _, userID := range memberIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := s.users[userID]; !ok {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		members = append(members, userID)
	}
	return members, nil
}

func (s *Service) appConfigForCreator(creatorID string) (AppConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.appConfigForCreatorLocked(creatorID)
}

func (s *Service) appConfigForCreatorLocked(creatorID string) (AppConfig, error) {
	return s.managerAppConfigLocked()
}

func (s *Service) appConfigForSenderLocked(senderID string) (AppConfig, error) {
	if app, ok := s.appConfigByIDLocked(senderID); ok {
		return validateAppConfig(app, senderID)
	}
	return s.managerAppConfigLocked()
}

func (s *Service) appConfigForMentionLocked(mention string) (AppConfig, error) {
	if app, ok := s.appConfigByIDLocked(mention); ok {
		return validateAppConfig(app, mention)
	}
	return AppConfig{}, fmt.Errorf("feishu app is not configured for mention %q", mention)
}

func (s *Service) participantMentionOpenIDLocked(participantID string) (string, bool) {
	participantID = strings.TrimSpace(participantID)
	if participantID == "" || s.configProvider == nil {
		return "", false
	}
	openID, ok := s.configProvider.MentionOpenID(participantID)
	openID = strings.TrimSpace(openID)
	return openID, ok && openID != ""
}

func (s *Service) managerAppConfigLocked() (AppConfig, error) {
	app, ok := s.appConfigByIDLocked(feishuManagerBotID)
	if !ok {
		return AppConfig{}, fmt.Errorf("feishu app is not configured for %q", feishuManagerBotID)
	}
	return validateAppConfig(app, feishuManagerBotID)
}

func (s *Service) appConfigByIDLocked(participantID string) (AppConfig, bool) {
	participantID = strings.TrimSpace(participantID)
	if participantID == "" {
		return AppConfig{}, false
	}
	if s.configProvider != nil {
		return s.configProvider.BotConfig(participantID)
	}
	app, ok := s.apps[participantID]
	return app, ok
}

func (s *Service) defaultAdminOpenID(app AppConfig) string {
	provider := s.configProviderSnapshot()
	if provider != nil {
		if openID, ok := provider.DefaultAdminOpenID(); ok {
			return strings.TrimSpace(openID)
		}
		if snapshot := provider.Snapshot(); strings.TrimSpace(snapshot.AdminOpenID) != "" {
			return strings.TrimSpace(snapshot.AdminOpenID)
		}
	}
	return strings.TrimSpace(app.AdminOpenID)
}

func validateAppConfig(app AppConfig, ownerID string) (AppConfig, error) {
	if strings.TrimSpace(app.AppID) == "" {
		return AppConfig{}, fmt.Errorf("feishu app_id is required for %q", ownerID)
	}
	if strings.TrimSpace(app.AppSecret) == "" {
		return AppConfig{}, fmt.Errorf("feishu app_secret is required for %q", ownerID)
	}
	return AppConfig{
		AppID:       strings.TrimSpace(app.AppID),
		AppSecret:   strings.TrimSpace(app.AppSecret),
		AdminOpenID: strings.TrimSpace(app.AdminOpenID),
	}, nil
}

func (s *Service) appConfigForRoom(roomID string) (AppConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.managerAppConfigLocked()
}

func (s *Service) appIDForMemberLocked(memberID string) (string, error) {
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return "", fmt.Errorf("member_id is required")
	}
	app, ok := s.appConfigByIDLocked(memberID)
	if !ok {
		return "", fmt.Errorf("feishu app is not configured for bot %q", memberID)
	}
	appID := strings.TrimSpace(app.AppID)
	if appID == "" {
		return "", fmt.Errorf("feishu app_id is required for bot %q", memberID)
	}
	return appID, nil
}

func (s *Service) appIDsForMembers(memberIDs []string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	appIDs := make([]string, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		memberID = strings.TrimSpace(memberID)
		if memberID == "" {
			continue
		}
		appID, err := s.appIDForMemberLocked(memberID)
		if err != nil {
			return nil, err
		}
		appIDs = append(appIDs, appID)
	}
	return appIDs, nil
}

func normalizeNonEmptyStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizeMembers(creatorID string, memberIDs []string) []string {
	seen := map[string]struct{}{creatorID: {}}
	members := []string{creatorID}
	for _, userID := range memberIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		members = append(members, userID)
	}
	return members
}

func feishuRequestUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("csgclaw-%d", time.Now().UnixNano())
	}
	return "csgclaw-" + hex.EncodeToString(b[:])
}

func normalizeUser(user im.User) im.User {
	user.ID = strings.TrimSpace(user.ID)
	user.Name = strings.TrimSpace(user.Name)
	user.Role = strings.ToLower(strings.TrimSpace(user.Role))
	if user.Name == "" {
		user.Name = user.ID
	}
	if user.Role == "" {
		user.Role = "member"
	}
	if user.Avatar == "" {
		user.Avatar = initials(user.Name)
	}
	if user.AccentHex == "" {
		user.AccentHex = accentHexForID(user.ID)
	}
	return user
}

func cloneRoom(room im.Room) im.Room {
	room.Members = append([]string(nil), room.Members...)
	room.Messages = append([]im.Message(nil), room.Messages...)
	return room
}

func deriveHandle(name, fallback string) string {
	source := strings.ToLower(strings.TrimSpace(name))
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(fallback))
	}
	var b strings.Builder
	for _, r := range source {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	handle := strings.Trim(b.String(), "-._")
	if handle == "" {
		return strings.ToLower(strings.TrimSpace(fallback))
	}
	return handle
}

func initials(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "FS"
	}
	parts := strings.Fields(name)
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		r := []rune(part)[0]
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		b.WriteRune(r)
		if b.Len() >= 2 {
			break
		}
	}
	if b.Len() == 0 {
		return "FS"
	}
	return b.String()
}

func accentHexForID(id string) string {
	palette := []string{"#0f766e", "#2563eb", "#7c3aed", "#dc2626", "#ca8a04", "#16a34a"}
	sum := 0
	for _, r := range id {
		sum += int(r)
	}
	return palette[sum%len(palette)]
}

func formatMembers(n int) string {
	if n == 1 {
		return "1 member"
	}
	return fmt.Sprintf("%d members", n)
}

func (s *Service) SetConfigProvider(provider Provider) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.configProvider = provider
	if provider != nil {
		s.apps = AppsFromSnapshot(provider.Snapshot())
	} else {
		s.apps = nil
	}
	s.mu.Unlock()
}

func (s *Service) ConfigProvider() Provider {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configProvider
}

func (s *Service) configProviderSnapshot() Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configProvider
}
