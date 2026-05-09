package apitypes

type FeishuConfigRequest struct {
	BotID       string `json:"bot_id,omitempty"`
	AppID       string `json:"app_id"`
	AppSecret   string `json:"app_secret"`
	AdminOpenID string `json:"admin_open_id,omitempty"`
	Reload      *bool  `json:"reload,omitempty"`
}

type FeishuConfigResponse struct {
	BotID       string `json:"bot_id"`
	Configured  bool   `json:"configured"`
	AppID       string `json:"app_id,omitempty"`
	AppSecret   string `json:"app_secret"`
	AdminOpenID string `json:"admin_open_id,omitempty"`
	Reloaded    bool   `json:"reloaded,omitempty"`
}

type FeishuConfigReloadResponse struct {
	Status     string   `json:"status"`
	FeishuBots []string `json:"feishu_bots"`
}
