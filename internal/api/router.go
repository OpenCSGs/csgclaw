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
				r.Route("/profile", func(r chi.Router) {
					r.Get("/", h.getAgentProfile)
					r.Put("/", h.updateAgentProfile)
				})
				r.Post("/recreate", h.recreateAgent)
			})
		})
		r.Route("/hub/templates", func(r chi.Router) {
			r.Get("/", h.listHubTemplates)
			r.Post("/", h.createHubTemplate)
			r.Get("/{id}", h.getHubTemplate)
			r.Get("/{id}/workspace/file", h.getHubTemplateWorkspaceFile)
		})
		r.Route("/cliproxy/auth", func(r chi.Router) {
			r.Get("/status", h.handleCLIProxyAuthStatus)
			r.Post("/login", h.handleCLIProxyAuthLogin)
		})
		r.Post("/agent-profiles/models", h.handleAgentProfileModels)
		r.Get("/agent-profile-defaults", h.handleAgentProfileDefaults)
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
	})
}

func (h *Handler) registerChannelRoutes(router chi.Router) {
	router.Route("/api/v1/channels", func(r chi.Router) {
		r.Route("/csgclaw", func(r chi.Router) {
			r.Route("/bots", func(r chi.Router) {
				r.Get("/", h.listBots)
				r.Post("/", h.createBot)
			})
			r.Route("/bots/{id}", func(r chi.Router) {
				r.Delete("/", h.deleteBot)
			})
			r.Route("/users", func(r chi.Router) {
				r.Get("/", h.listUsers)
				r.Post("/", h.createUser)
				r.Delete("/{id}", h.deleteCsgclawUser)
			})
			r.Route("/rooms", func(r chi.Router) {
				r.Get("/", h.listRooms)
				r.Post("/", h.createRoom)
				r.Route("/{id}", func(r chi.Router) {
					r.Delete("/", h.deleteCsgclawRoom)
					r.Route("/members", func(r chi.Router) {
						r.Get("/", h.listCsgclawRoomMembers)
						r.Post("/", h.addCsgclawRoomMembers)
					})
				})
			})
			r.Route("/messages", func(r chi.Router) {
				r.Get("/", h.listMessages)
				r.Post("/", h.createMessage)
			})
		})
		r.Route("/feishu", func(r chi.Router) {
			r.Route("/bots", func(r chi.Router) {
				r.Get("/", h.listBots)
				r.Post("/", h.createBot)
			})
			r.Route("/config", func(r chi.Router) {
				r.Get("/", h.getFeishuConfig)
				r.Put("/", h.updateFeishuConfig)
				r.Post("/", h.reloadFeishuConfig)
			})
			r.Route("/bots/{id}", func(r chi.Router) {
				r.Delete("/", h.deleteBot)
				r.Get("/events", h.getFeishuBotEvents)
			})
			r.Route("/users", func(r chi.Router) {
				r.Get("/", h.listFeishuUsers)
				r.Post("/", h.createFeishuUser)
				r.Delete("/{id}", h.deleteFeishuUser)
			})
			r.Route("/rooms", func(r chi.Router) {
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
			r.Route("/messages", func(r chi.Router) {
				r.Get("/", h.listFeishuMessages)
				r.Post("/", h.createFeishuMessage)
			})
		})
		r.Route("/{channel}/bots", func(r chi.Router) {
			r.Get("/", h.listBots)
			r.Post("/", h.createBot)
		})
		r.Route("/{channel}/bots/{id}", func(r chi.Router) {
			r.Delete("/", h.deleteBot)
		})
	})
}
