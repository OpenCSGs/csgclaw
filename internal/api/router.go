package api

import "github.com/go-chi/chi/v5"

func (h *Handler) Routes() chi.Router {
	router := chi.NewRouter()
	h.registerCoreRoutes(router)
	h.registerChannelRoutes(router)
	return router
}

func (h *Handler) registerCoreRoutes(router chi.Router) {
	router.Get("/healthz", h.handleHealthz)
	router.Route("/api/v1", func(r chi.Router) {
		r.Get("/version", h.getVersion)
		r.Route("/upgrade", func(r chi.Router) {
			r.Get("/status", h.getUpgradeStatus)
			r.Post("/apply", h.createUpgradeApply)
		})
		r.Route("/agent-runtimes", func(r chi.Router) {
			r.Get("/", h.listAgentRuntimes)
			r.Post("/{name}/install", h.installAgentRuntime)
		})
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", h.listAgents)
			r.Post("/", h.createAgent)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.getAgent)
				r.Patch("/", h.updateAgent)
				r.Delete("/", h.deleteAgent)
				r.Post("/start", h.startAgent)
				r.Post("/stop", h.stopAgent)
				r.Get("/logs", h.getAgentLogs)
				r.Get("/workspace", h.handleAgentWorkspace)
				r.Get("/workspace/file", h.handleAgentWorkspaceFile)
				r.Get("/skills", h.handleAgentSkills)
				r.Post("/skills:batchAdd", h.handleAgentSkillsBatchAdd)
				r.Get("/skills/file", h.handleAgentSkillsFile)
				r.Delete("/skills/{name}", h.handleAgentSkillDelete)
				r.Route("/profile", func(r chi.Router) {
					r.Get("/", h.getAgentProfile)
					r.Put("/", h.updateAgentProfile)
				})
				r.Route("/llm", func(r chi.Router) {
					r.Get("/models", h.getAgentLLMModels)
					r.Get("/v1/models", h.getAgentLLMModels)
					r.Post("/chat/completions", h.createAgentLLMChatCompletions)
					r.Post("/v1/chat/completions", h.createAgentLLMChatCompletions)
					r.Post("/responses", h.createAgentLLMResponses)
					r.Post("/v1/responses", h.createAgentLLMResponses)
					r.Get("/responses", h.getAgentLLMResponsesWebsocket)
					r.Get("/v1/responses", h.getAgentLLMResponsesWebsocket)
				})
				r.Post("/connectors/{provider}/credential", h.handleAgentConnectorCredential)
				r.Post("/recreate", h.recreateAgent)
				r.Post("/upgrade", h.upgradeAgent)
			})
		})
		r.Route("/hub/templates", func(r chi.Router) {
			r.Get("/", h.listHubTemplates)
			r.Post("/", h.createHubTemplate)
			r.Get("/{id}", h.getHubTemplateByID)
			r.Delete("/{id}", h.deleteHubTemplateByID)
			r.Get("/{id}/workspace", h.getHubTemplateWorkspaceByID)
			r.Get("/{id}/workspace/file", h.getHubTemplateWorkspaceFileByID)
		})
		r.Get("/skills", h.listSkills)
		r.Post("/skills:upload", h.handleSkillUpload)
		r.Post("/skills:install", h.handleSkillInstall)
		r.Delete("/skills/{name}", h.deleteSkill)
		r.Get("/skills/tree", h.getSkillTree)
		r.Get("/skills/file", h.getSkillFile)
		r.Route("/model-providers", func(r chi.Router) {
			r.Get("/", h.handleModelProviders)
			r.Post("/", h.handleModelProviders)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", h.handleModelProviderByID)
				r.Delete("/", h.handleModelProviderByID)
				r.Post("/check", h.checkModelProvider)
			})
		})
		r.Route("/cliproxy/auth", func(r chi.Router) {
			r.Get("/status", h.handleCLIProxyAuthStatus)
			r.Post("/login", h.handleCLIProxyAuthLogin)
			r.Post("/logout", h.handleCLIProxyAuthLogout)
		})
		r.Route("/auth", func(r chi.Router) {
			r.Get("/status", h.handleAuthStatus)
			r.Get("/callback", h.handleAuthCallback)
			r.Post("/login", h.handleAuthLogin)
			r.Post("/logout", h.handleAuthLogout)
		})
		r.Route("/connectors", func(r chi.Router) {
			r.Get("/", h.handleConnectors)
			r.Route("/github", func(r chi.Router) {
				r.Get("/", h.handleGitHubConnector)
				r.Put("/config", h.handleGitHubConnectorConfig)
				r.Post("/app/install/start", h.handleGitHubConnectorAppInstallStart)
				r.Get("/oauth/start", h.handleGitHubConnectorOAuthStart)
				r.Post("/oauth/start", h.handleGitHubConnectorOAuthStart)
				r.Get("/oauth/callback", h.handleGitHubConnectorOAuthCallback)
				r.Post("/disconnect", h.handleGitHubConnectorDisconnect)
				r.Post("/credential", h.handleGitHubConnectorCredential)
			})
		})
		r.Post("/agent-profiles/models", h.handleAgentProfileModels)
		r.Get("/agent-profile-defaults", h.handleAgentProfileDefaults)
		r.Post("/local/directory-picker", h.handleLocalDirectoryPicker)
		r.Get("/agents/image-candidates", h.listAgentImageCandidates)
		r.Route("/config/bootstrap", func(r chi.Router) {
			r.Get("/", h.getBootstrapConfig)
			r.Put("/", h.updateBootstrapConfig)
		})
		r.Route("/server", func(r chi.Router) {
			r.Get("/config", h.getServerConfig)
			r.Put("/config", h.updateServerConfig)
			r.Post("/restart", h.postServerRestart)
			r.Get("/restart/status", h.getServerRestartStatus)
		})
		r.Get("/bootstrap", h.getIMBootstrap)
		r.Get("/events", h.getIMEvents)
		r.Route("/rooms", func(r chi.Router) {
			r.Get("/", h.listRooms)
			r.Post("/", h.createRoom)
			r.Post("/{id}:clearMessages", h.clearRoomMessages)
			r.Route("/{id}", func(r chi.Router) {
				r.Delete("/", h.deleteRoom)
				r.Get("/threads", h.listThreads)
				r.Post("/threads", h.createThread)
				r.Get("/threads/{thread_id}", h.getThread)
				r.Get("/relations/{event_id}/m.thread", h.listThreadRelations)
				r.Route("/members", func(r chi.Router) {
					r.Get("/", h.listRoomMembers)
					r.Post("/", h.addRoomMembers)
					r.Delete("/{member_id}", h.deleteRoomMember)
				})
			})
			r.Post("/invite", h.createIMRoomMembersInvite)
		})
		r.Route("/messages", func(r chi.Router) {
			r.Get("/", h.listMessages)
			r.Post("/", h.createMessage)
		})
		r.Route("/teams", func(r chi.Router) {
			r.Get("/", h.listTeams)
			r.Post("/", h.createTeam)
			r.Post("/tasks/claim-next", h.claimNextTask)
			r.Route("/{team_id}", func(r chi.Router) {
				r.Get("/", h.getTeam)
				r.Patch("/", h.updateTeam)
				r.Delete("/", h.deleteTeam)
				r.Route("/tasks", func(r chi.Router) {
					r.Get("/", h.listTeamTasks)
					r.Post("/batch", h.createTeamTasksBatch)
					r.Post("/claim-next", h.claimNextTask)
					r.Route("/{task_id}", func(r chi.Router) {
						r.Post("/plan", h.planTeamTask)
						r.Post("/start", h.startTeamTask)
						r.Post("/claim", h.claimTeamTask)
						r.Patch("/", h.updateTeamTask)
						r.Post("/assign", h.assignTeamTask)
					})
				})
				r.Route("/approvals", func(r chi.Router) {
					r.Get("/", h.listTeamApprovals)
					r.Post("/", h.createTeamApproval)
					r.Post("/{approval_id}/resolve", h.resolveTeamApproval)
				})
				r.Get("/events", h.listTeamEvents)
			})
		})
		r.Route("/agent-tasks", func(r chi.Router) {
			r.Get("/", h.listAgentTasks)
			r.Post("/", h.createAgentTask)
			r.Route("/{task_id}", func(r chi.Router) {
				r.Post("/claim", h.claimAgentTask)
				r.Patch("/", h.updateAgentTask)
			})
		})
		r.Route("/scheduled-tasks", func(r chi.Router) {
			r.Get("/", h.listScheduledTasks)
			r.Post("/", h.createScheduledTask)
			r.Route("/{scheduled_task_id}", func(r chi.Router) {
				r.Patch("/", h.updateScheduledTask)
				r.Delete("/", h.deleteScheduledTask)
				r.Get("/runs", h.listScheduledTaskRuns)
				r.Post("/run-now", h.runScheduledTaskNow)
			})
		})
		r.Get("/tasks", h.listGlobalTasks)
	})
}

func (h *Handler) registerChannelRoutes(router chi.Router) {
	router.Patch("/api/v1/channels/csgclaw/users/{id}", h.updateCsgclawUser)
	router.Patch("/api/v1/channels/csgclaw/users/{id}/", h.updateCsgclawUser)
	router.Delete("/api/v1/channels/csgclaw/users/{id}", h.deleteCsgclawUser)
	router.Delete("/api/v1/channels/csgclaw/users/{id}/", h.deleteCsgclawUser)

	router.Route("/api/v1/channels", func(r chi.Router) {
		r.Route("/{channel}/participants", func(r chi.Router) {
			r.Get("/", h.listParticipants)
			r.Post("/", h.createParticipant)
		})
		r.Route("/{channel}/participants/{id}", func(r chi.Router) {
			r.Get("/", h.handleParticipantByID)
			r.Patch("/", h.handleParticipantByID)
			r.Delete("/", h.handleParticipantByID)
			r.Get("/events", h.getParticipantEvents)
			r.Post("/messages", h.createParticipantMessage)
			r.Post("/notifications", h.createParticipantNotification)
		})
		r.Post("/{channel}/activities/{activity_id}:decide", h.handleChannelActivityDecision)

		// CSGClaw channel IM routes.
		r.Route("/csgclaw/users", func(r chi.Router) {
			r.Get("/", h.listUsers)
			r.Post("/", h.createUser)
			r.Patch("/{id}", h.updateCsgclawUser)
			r.Patch("/{id}/", h.updateCsgclawUser)
			r.Delete("/{id}", h.deleteCsgclawUser)
			r.Delete("/{id}/", h.deleteCsgclawUser)
		})
		r.Route("/csgclaw/rooms", func(r chi.Router) {
			r.Get("/", h.listRooms)
			r.Post("/", h.createRoom)
			r.Route("/{id}", func(r chi.Router) {
				r.Delete("/", h.deleteCsgclawRoom)
				r.Get("/threads", h.listThreads)
				r.Post("/threads", h.createThread)
				r.Get("/threads/{thread_id}", h.getThread)
				r.Get("/relations/{event_id}/m.thread", h.listThreadRelations)
				r.Route("/members", func(r chi.Router) {
					r.Get("/", h.listCsgclawRoomMembers)
					r.Post("/", h.addCsgclawRoomMembers)
				})
			})
		})
		r.Route("/csgclaw/messages", func(r chi.Router) {
			r.Get("/", h.listMessages)
			r.Post("/", h.createMessage)
		})

		// Feishu channel routes.
		r.Route("/feishu/registrations", func(r chi.Router) {
			r.Post("/", h.createFeishuRegistration)
			r.Get("/{registration_id}", h.getFeishuRegistration)
			r.Post("/{registration_id}:finalize", h.finalizeFeishuRegistration)
		})
		r.Route("/feishu/users", func(r chi.Router) {
			r.Get("/", h.listFeishuUsers)
			r.Post("/", h.createFeishuUser)
			r.Delete("/{id}", h.deleteFeishuUser)
		})
		r.Route("/feishu/rooms", func(r chi.Router) {
			r.Get("/", h.listFeishuRooms)
			r.Post("/", h.createFeishuRoom)
			r.Route("/{id}", func(r chi.Router) {
				r.Delete("/", h.deleteFeishuRoom)
				r.Route("/members", func(r chi.Router) {
					r.Get("/", h.listFeishuRoomMembers)
					r.Post("/", h.addFeishuRoomMembers)
				})
			})
		})
		r.Route("/feishu/messages", func(r chi.Router) {
			r.Get("/", h.listFeishuMessages)
			r.Post("/", h.createFeishuMessage)
		})
	})
}
