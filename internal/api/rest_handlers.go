package api

import "net/http"

func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) { h.handleVersion(w, r) }
func (h *Handler) getUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	h.handleUpgradeStatus(w, r)
}
func (h *Handler) createUpgradeApply(w http.ResponseWriter, r *http.Request) {
	h.handleUpgradeApply(w, r)
}
func (h *Handler) listParticipants(w http.ResponseWriter, r *http.Request) {
	h.handleParticipants(w, r)
}
func (h *Handler) createParticipant(w http.ResponseWriter, r *http.Request) {
	h.handleParticipants(w, r)
}
func (h *Handler) handleParticipantByID(w http.ResponseWriter, r *http.Request) {
	h.handleParticipantByIDPath(w, r)
}
func (h *Handler) getParticipantEvents(w http.ResponseWriter, r *http.Request) {
	h.handleParticipantEvents(w, r)
}
func (h *Handler) createParticipantMessage(w http.ResponseWriter, r *http.Request) {
	h.handleParticipantMessage(w, r)
}
func (h *Handler) createParticipantNotification(w http.ResponseWriter, r *http.Request) {
	h.pushNotificationParticipant(w, r)
}
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
func (h *Handler) getAgentLLMModels(w http.ResponseWriter, r *http.Request) {
	h.handleAgentLLMModelsByID(w, r)
}
func (h *Handler) createAgentLLMChatCompletions(w http.ResponseWriter, r *http.Request) {
	h.handleAgentLLMChatCompletionsByID(w, r)
}
func (h *Handler) createAgentLLMResponses(w http.ResponseWriter, r *http.Request) {
	h.handleAgentLLMResponsesByID(w, r)
}
func (h *Handler) getAgentLLMResponsesWebsocket(w http.ResponseWriter, r *http.Request) {
	h.handleAgentLLMResponsesWebsocketByID(w, r)
}
func (h *Handler) listHubTemplates(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplates(w, r)
}
func (h *Handler) createHubTemplate(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplates(w, r)
}
func (h *Handler) listSkills(w http.ResponseWriter, r *http.Request)   { h.handleSkills(w, r) }
func (h *Handler) deleteSkill(w http.ResponseWriter, r *http.Request)  { h.handleSkillByName(w, r) }
func (h *Handler) getSkillTree(w http.ResponseWriter, r *http.Request) { h.handleSkillTree(w, r) }
func (h *Handler) getSkillFile(w http.ResponseWriter, r *http.Request) { h.handleSkillFile(w, r) }
func (h *Handler) getHubTemplateByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateByID(w, r)
}
func (h *Handler) deleteHubTemplateByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateByID(w, r)
}
func (h *Handler) getHubTemplateWorkspaceFileByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateWorkspaceFileByID(w, r)
}
func (h *Handler) getHubTemplateWorkspaceByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateWorkspaceByID(w, r)
}
func (h *Handler) getBootstrapConfig(w http.ResponseWriter, r *http.Request) {
	h.handleBootstrapConfig(w, r)
}
func (h *Handler) updateBootstrapConfig(w http.ResponseWriter, r *http.Request) {
	h.handleBootstrapConfig(w, r)
}
func (h *Handler) getServerConfig(w http.ResponseWriter, r *http.Request) { h.handleServerConfig(w, r) }
func (h *Handler) updateServerConfig(w http.ResponseWriter, r *http.Request) {
	h.handleServerConfig(w, r)
}
func (h *Handler) postServerRestart(w http.ResponseWriter, r *http.Request) {
	h.handleServerRestart(w, r)
}
func (h *Handler) getServerRestartStatus(w http.ResponseWriter, r *http.Request) {
	h.handleServerRestartStatus(w, r)
}
func (h *Handler) getIMBootstrap(w http.ResponseWriter, r *http.Request) { h.handleIMBootstrap(w, r) }
func (h *Handler) getIMEvents(w http.ResponseWriter, r *http.Request)    { h.handleIMEvents(w, r) }
func (h *Handler) listRooms(w http.ResponseWriter, r *http.Request)      { h.handleRooms(w, r) }
func (h *Handler) createRoom(w http.ResponseWriter, r *http.Request)     { h.handleCreateRoom(w, r) }
func (h *Handler) deleteRoom(w http.ResponseWriter, r *http.Request)     { h.handleRoomByID(w, r) }
func (h *Handler) clearRoomMessages(w http.ResponseWriter, r *http.Request) {
	h.handleClearRoomMessages(w, r)
}
func (h *Handler) listThreads(w http.ResponseWriter, r *http.Request)  { h.handleThreadsByRoomID(w, r) }
func (h *Handler) createThread(w http.ResponseWriter, r *http.Request) { h.handleThreadsByRoomID(w, r) }
func (h *Handler) getThread(w http.ResponseWriter, r *http.Request)    { h.handleThreadByID(w, r) }
func (h *Handler) listThreadRelations(w http.ResponseWriter, r *http.Request) {
	h.handleThreadRelationsByID(w, r)
}
func (h *Handler) listRoomMembers(w http.ResponseWriter, r *http.Request) {
	h.handleRoomMembersByIDPath(w, r)
}
func (h *Handler) addRoomMembers(w http.ResponseWriter, r *http.Request) {
	h.handleRoomMembersByIDPath(w, r)
}
func (h *Handler) deleteRoomMember(w http.ResponseWriter, r *http.Request) {
	h.handleRoomMemberDeletePath(w, r)
}
func (h *Handler) createIMRoomMembersInvite(w http.ResponseWriter, r *http.Request) {
	h.handleIMRoomMembers(w, r)
}
func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request)     { h.handleUsers(w, r) }
func (h *Handler) createUser(w http.ResponseWriter, r *http.Request)    { h.handleCreateUser(w, r) }
func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request)    { h.handleUserByID(w, r) }
func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request)  { h.handleMessages(w, r) }
func (h *Handler) createMessage(w http.ResponseWriter, r *http.Request) { h.handleCreateMessage(w, r) }
func (h *Handler) listTeams(w http.ResponseWriter, r *http.Request)     { h.handleListTeams(w, r) }
func (h *Handler) createTeam(w http.ResponseWriter, r *http.Request)    { h.handleCreateTeam(w, r) }
func (h *Handler) getTeam(w http.ResponseWriter, r *http.Request)       { h.handleGetTeam(w, r) }
func (h *Handler) updateTeam(w http.ResponseWriter, r *http.Request)    { h.handleUpdateTeam(w, r) }
func (h *Handler) deleteTeam(w http.ResponseWriter, r *http.Request)    { h.handleDeleteTeam(w, r) }
func (h *Handler) listTeamTasks(w http.ResponseWriter, r *http.Request) { h.handleListTeamTasks(w, r) }
func (h *Handler) createTeamTasksBatch(w http.ResponseWriter, r *http.Request) {
	h.handleCreateTeamTasksBatch(w, r)
}
func (h *Handler) claimNextTask(w http.ResponseWriter, r *http.Request) {
	h.handleClaimNextTask(w, r)
}
func (h *Handler) claimTeamTask(w http.ResponseWriter, r *http.Request) {
	h.handleClaimTeamTask(w, r)
}
func (h *Handler) updateTeamTask(w http.ResponseWriter, r *http.Request) {
	h.handleUpdateTeamTask(w, r)
}
func (h *Handler) planTeamTask(w http.ResponseWriter, r *http.Request) { h.handlePlanTeamTask(w, r) }
func (h *Handler) startTeamTask(w http.ResponseWriter, r *http.Request) {
	h.handleStartTeamTask(w, r)
}
func (h *Handler) assignTeamTask(w http.ResponseWriter, r *http.Request) {
	h.handleAssignTeamTask(w, r)
}
func (h *Handler) listTeamApprovals(w http.ResponseWriter, r *http.Request) {
	h.handleListTeamApprovals(w, r)
}
func (h *Handler) createTeamApproval(w http.ResponseWriter, r *http.Request) {
	h.handleCreateTeamApproval(w, r)
}
func (h *Handler) resolveTeamApproval(w http.ResponseWriter, r *http.Request) {
	h.handleResolveTeamApproval(w, r)
}
func (h *Handler) listTeamEvents(w http.ResponseWriter, r *http.Request) {
	h.handleListTeamEvents(w, r)
}
func (h *Handler) listAgentTasks(w http.ResponseWriter, r *http.Request) {
	h.handleListAgentTasks(w, r)
}
func (h *Handler) createAgentTask(w http.ResponseWriter, r *http.Request) {
	h.handleCreateAgentTask(w, r)
}
func (h *Handler) claimAgentTask(w http.ResponseWriter, r *http.Request) {
	h.handleClaimAgentTask(w, r)
}
func (h *Handler) updateAgentTask(w http.ResponseWriter, r *http.Request) {
	h.handleUpdateAgentTask(w, r)
}
func (h *Handler) listGlobalTasks(w http.ResponseWriter, r *http.Request) {
	h.handleListGlobalTasks(w, r)
}

func (h *Handler) listScheduledTasks(w http.ResponseWriter, r *http.Request) {
	h.handleListScheduledTasks(w, r)
}

func (h *Handler) createScheduledTask(w http.ResponseWriter, r *http.Request) {
	h.handleCreateScheduledTask(w, r)
}

func (h *Handler) updateScheduledTask(w http.ResponseWriter, r *http.Request) {
	h.handleUpdateScheduledTask(w, r)
}

func (h *Handler) deleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteScheduledTask(w, r)
}

func (h *Handler) listScheduledTaskRuns(w http.ResponseWriter, r *http.Request) {
	h.handleListScheduledTaskRuns(w, r)
}

func (h *Handler) runScheduledTaskNow(w http.ResponseWriter, r *http.Request) {
	h.handleRunScheduledTaskNow(w, r)
}
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
