package api

import "github.com/go-chi/chi/v5"

func (h *Handler) Routes() chi.Router {
	router := chi.NewRouter()
	h.registerCoreRoutes(router)
	h.registerChannelRoutes(router)
	h.registerBotCompatibilityRoutes(router)
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
				r.Route("/profile", func(r chi.Router) {
					r.Get("/", h.getAgentProfile)
					r.Put("/", h.updateAgentProfile)
				})
				r.Post("/recreate", h.recreateAgent)
				r.Post("/upgrade", h.upgradeAgent)
			})
		})
		r.Route("/hub/templates", func(r chi.Router) {
			r.Get("/", h.listHubTemplates)
			r.Post("/", h.createHubTemplate)
			r.Get("/{id}", h.getHubTemplateByID)
			r.Delete("/{id}", h.deleteHubTemplateByID)
			r.Get("/{id}/workspace/file", h.getHubTemplateWorkspaceFileByID)
		})
		r.Route("/cliproxy/auth", func(r chi.Router) {
			r.Get("/status", h.handleCLIProxyAuthStatus)
			r.Post("/login", h.handleCLIProxyAuthLogin)
		})
		r.Post("/agent-profiles/models", h.handleAgentProfileModels)
		r.Get("/agent-profile-defaults", h.handleAgentProfileDefaults)
		r.Get("/agents/image-candidates", h.listAgentImageCandidates)
		r.Route("/config/bootstrap", func(r chi.Router) {
			r.Get("/", h.getBootstrapConfig)
			r.Put("/", h.updateBootstrapConfig)
		})
		r.Get("/bootstrap", h.getIMBootstrap)
		r.Get("/events", h.getIMEvents)
		r.Route("/rooms", func(r chi.Router) {
			r.Get("/", h.listRooms)
			r.Post("/", h.createRoom)
			r.Route("/{id}", func(r chi.Router) {
				r.Delete("/", h.deleteRoom)
				r.Get("/threads", h.listThreads)
				r.Post("/threads", h.createThread)
				r.Get("/threads/{thread_id}", h.getThread)
				r.Get("/relations/{event_id}/m.thread", h.listThreadRelations)
				r.Route("/members", func(r chi.Router) {
					r.Get("/", h.listRoomMembers)
					r.Post("/", h.addRoomMembers)
				})
			})
			r.Post("/invite", h.createIMRoomMembersInvite)
		})
		r.Route("/users", func(r chi.Router) {
			r.Get("/", h.listUsers)
			r.Post("/", h.createUser)
			r.Delete("/{id}", h.deleteUser)
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
				r.Route("/tasks", func(r chi.Router) {
					r.Get("/", h.listTeamTasks)
					r.Post("/batch", h.createTeamTasksBatch)
					r.Post("/claim-next", h.claimNextTask)
					r.Route("/{task_id}", func(r chi.Router) {
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
		r.Get("/tasks", h.listGlobalTasks)
	})
}

func (h *Handler) registerChannelRoutes(router chi.Router) {
	router.Route("/api/v1/channels", func(r chi.Router) {
		// Generic bot CRUD for all channels.
		r.Route("/{channel}/bots", func(r chi.Router) {
			r.Get("/", h.listBots)
			r.Post("/", h.createBot)
		})
		r.Route("/{channel}/bots/{id}", func(r chi.Router) {
			r.Get("/", h.handleBotByID)
			r.Patch("/", h.handleBotByID)
			r.Delete("/", h.deleteBot)
		})
		r.Post("/{channel}/activities/{activity_id}:decide", h.handleChannelActivityDecision)

		// Channel-specific bot operations (not exposed on generic /{channel}/bots/{id}).
		r.Post("/csgclaw/bots/{id}/notifications", h.pushNotificationBot)
		r.Get("/feishu/bots/{id}/events", h.getFeishuBotEvents)

		// CSGClaw channel IM routes (flat paths so /csgclaw/bots stays on generic CRUD).
		r.Route("/csgclaw/users", func(r chi.Router) {
			r.Get("/", h.listUsers)
			r.Post("/", h.createUser)
			r.Delete("/{id}", h.deleteCsgclawUser)
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

		// Feishu channel routes (flat paths; bot list/CRUD uses generic /{channel}/bots).
		r.Route("/feishu/config", func(r chi.Router) {
			r.Get("/", h.getFeishuConfig)
			r.Put("/", h.updateFeishuConfig)
			r.Post("/", h.reloadFeishuConfig)
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
