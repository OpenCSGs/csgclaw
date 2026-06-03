package api

import "net/http"

func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) { h.handleVersion(w, r) }
func (h *Handler) getUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	h.handleUpgradeStatus(w, r)
}
func (h *Handler) createUpgradeApply(w http.ResponseWriter, r *http.Request) {
	h.handleUpgradeApply(w, r)
}
func (h *Handler) listBots(w http.ResponseWriter, r *http.Request)     { h.handleBots(w, r) }
func (h *Handler) createBot(w http.ResponseWriter, r *http.Request)    { h.handleBots(w, r) }
func (h *Handler) deleteBot(w http.ResponseWriter, r *http.Request)    { h.handleBotByID(w, r) }
func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request)   { h.handleAgents(w, r) }
func (h *Handler) createAgent(w http.ResponseWriter, r *http.Request)  { h.handleAgents(w, r) }
func (h *Handler) getAgent(w http.ResponseWriter, r *http.Request)     { h.handleAgentByID(w, r) }
func (h *Handler) updateAgent(w http.ResponseWriter, r *http.Request)  { h.handleAgentByID(w, r) }
func (h *Handler) deleteAgent(w http.ResponseWriter, r *http.Request)  { h.handleAgentByID(w, r) }
func (h *Handler) startAgent(w http.ResponseWriter, r *http.Request)   { h.handleAgentStartByID(w, r) }
func (h *Handler) stopAgent(w http.ResponseWriter, r *http.Request)    { h.handleAgentStopByID(w, r) }
func (h *Handler) getAgentLogs(w http.ResponseWriter, r *http.Request) { h.handleAgentLogsByID(w, r) }
func (h *Handler) getAgentProfile(w http.ResponseWriter, r *http.Request) {
	h.handleAgentProfileByID(w, r)
}
func (h *Handler) updateAgentProfile(w http.ResponseWriter, r *http.Request) {
	h.handleAgentProfileByID(w, r)
}
func (h *Handler) recreateAgent(w http.ResponseWriter, r *http.Request) {
	h.handleAgentRecreateByID(w, r)
}
func (h *Handler) upgradeAgent(w http.ResponseWriter, r *http.Request) {
	h.handleAgentUpgradeByID(w, r)
}
func (h *Handler) listHubTemplates(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplates(w, r)
}
func (h *Handler) createHubTemplate(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplates(w, r)
}
func (h *Handler) getHubTemplateByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateByID(w, r)
}
func (h *Handler) deleteHubTemplateByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateByID(w, r)
}
func (h *Handler) getHubTemplateWorkspaceFileByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateWorkspaceFileByID(w, r)
}
func (h *Handler) getBootstrapConfig(w http.ResponseWriter, r *http.Request) {
	h.handleBootstrapConfig(w, r)
}
func (h *Handler) updateBootstrapConfig(w http.ResponseWriter, r *http.Request) {
	h.handleBootstrapConfig(w, r)
}
func (h *Handler) getIMBootstrap(w http.ResponseWriter, r *http.Request) { h.handleIMBootstrap(w, r) }
func (h *Handler) getIMEvents(w http.ResponseWriter, r *http.Request)    { h.handleIMEvents(w, r) }
func (h *Handler) listRooms(w http.ResponseWriter, r *http.Request)      { h.handleRooms(w, r) }
func (h *Handler) createRoom(w http.ResponseWriter, r *http.Request)     { h.handleCreateRoom(w, r) }
func (h *Handler) deleteRoom(w http.ResponseWriter, r *http.Request)     { h.handleRoomByID(w, r) }
func (h *Handler) listThreads(w http.ResponseWriter, r *http.Request)    { h.handleThreadsByRoomID(w, r) }
func (h *Handler) createThread(w http.ResponseWriter, r *http.Request)   { h.handleThreadsByRoomID(w, r) }
func (h *Handler) getThread(w http.ResponseWriter, r *http.Request)      { h.handleThreadByID(w, r) }
func (h *Handler) listThreadRelations(w http.ResponseWriter, r *http.Request) {
	h.handleThreadRelationsByID(w, r)
}
func (h *Handler) listRoomMembers(w http.ResponseWriter, r *http.Request) {
	h.handleRoomMembersByIDPath(w, r)
}
func (h *Handler) addRoomMembers(w http.ResponseWriter, r *http.Request) {
	h.handleRoomMembersByIDPath(w, r)
}
func (h *Handler) createIMRoomMembersInvite(w http.ResponseWriter, r *http.Request) {
	h.handleIMRoomMembers(w, r)
}
func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request)            { h.handleUsers(w, r) }
func (h *Handler) createUser(w http.ResponseWriter, r *http.Request)           { h.handleCreateUser(w, r) }
func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request)           { h.handleUserByID(w, r) }
func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request)         { h.handleMessages(w, r) }
func (h *Handler) createMessage(w http.ResponseWriter, r *http.Request)        { h.handleCreateMessage(w, r) }
func (h *Handler) listIMMessages(w http.ResponseWriter, r *http.Request)       { h.handleIMMessages(w, r) }
func (h *Handler) createIMMessage(w http.ResponseWriter, r *http.Request)      { h.handleIMMessages(w, r) }
func (h *Handler) listIMConversations(w http.ResponseWriter, r *http.Request)  { h.handleIMRooms(w, r) }
func (h *Handler) createIMConversation(w http.ResponseWriter, r *http.Request) { h.handleIMRooms(w, r) }
func (h *Handler) createIMConversationMembers(w http.ResponseWriter, r *http.Request) {
	h.handleIMRoomMembers(w, r)
}
func (h *Handler) listIMRooms(w http.ResponseWriter, r *http.Request)  { h.handleIMRooms(w, r) }
func (h *Handler) createIMRoom(w http.ResponseWriter, r *http.Request) { h.handleIMRooms(w, r) }
func (h *Handler) deleteCsgclawUser(w http.ResponseWriter, r *http.Request) {
	h.handleCsgclawUserByID(w, r)
}
func (h *Handler) deleteCsgclawRoom(w http.ResponseWriter, r *http.Request) {
	h.handleCsgclawRoomByID(w, r)
}
func (h *Handler) listCsgclawRoomMembers(w http.ResponseWriter, r *http.Request) {
	h.handleCsgclawRoomMembersByID(w, r)
}
func (h *Handler) addCsgclawRoomMembers(w http.ResponseWriter, r *http.Request) {
	h.handleCsgclawRoomMembersByID(w, r)
}
func (h *Handler) getFeishuConfig(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuConfigGet(w, r)
}
func (h *Handler) updateFeishuConfig(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuConfigPut(w, r)
}
func (h *Handler) reloadFeishuConfig(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuConfigReload(w)
}
func (h *Handler) getFeishuBotEvents(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuBotByID(w, r)
}
func (h *Handler) listFeishuUsers(w http.ResponseWriter, r *http.Request)  { h.handleFeishuUsers(w, r) }
func (h *Handler) createFeishuUser(w http.ResponseWriter, r *http.Request) { h.handleFeishuUsers(w, r) }
func (h *Handler) deleteFeishuUser(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuUserByID(w, r)
}
func (h *Handler) listFeishuRooms(w http.ResponseWriter, r *http.Request)  { h.handleFeishuRooms(w, r) }
func (h *Handler) createFeishuRoom(w http.ResponseWriter, r *http.Request) { h.handleFeishuRooms(w, r) }
func (h *Handler) deleteFeishuRoom(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuRoomByID(w, r)
}
func (h *Handler) listFeishuRoomMembers(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuRoomMembersByID(w, r)
}
func (h *Handler) addFeishuRoomMembers(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuRoomMembersByID(w, r)
}
func (h *Handler) listFeishuMessages(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuMessages(w, r)
}
func (h *Handler) createFeishuMessage(w http.ResponseWriter, r *http.Request) {
	h.handleFeishuMessages(w, r)
}
