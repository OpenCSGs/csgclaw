// Package modelpreset owns the public model service directory and transport defaults.
package modelpreset

import "strings"

const (
	Responses         = "responses"
	ChatCompletions   = "chat_completions"
	AnthropicMessages = "anthropic_messages"
)

type Region struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	BaseURL         string            `json:"base_url"`
	KeyURL          string            `json:"key_url"`
	ProtocolURLs    map[string]string `json:"protocol_urls,omitempty"`
	DefaultProtocol string            `json:"default_protocol"`
}

type Preset struct {
	ID        string   `json:"id"`
	Brand     string   `json:"brand"`
	BrandName string   `json:"brand_name"`
	Name      string   `json:"name"`
	Avatar    string   `json:"avatar"`
	Aliases   []string `json:"aliases"`
	Protocol  string   `json:"api_protocol"`
	DocsURL   string   `json:"docs_url"`
	Regions   []Region `json:"regions"`
	Models    []string `json:"models"`
}

func region(id, name, base, key string) Region {
	return Region{ID: id, Name: name, BaseURL: base, KeyURL: key}
}

var catalog = []Preset{
	{"openai", "openai", "OpenAI", "OpenAI API", "model-providers/openai-api.svg", []string{"GPT"}, Responses, "https://developers.openai.com/api/reference/overview", []Region{region("global", "Global", "https://api.openai.com/v1", "https://platform.openai.com/api-keys")}, nil},
	{"claude-api", "claude", "Claude", "Claude API", "model-providers/claude-api.svg", []string{"Anthropic"}, AnthropicMessages, "https://platform.claude.com/docs/en/api/overview", []Region{region("global", "Global", "https://api.anthropic.com/v1", "https://platform.claude.com/settings/keys")}, nil},
	{"kimi", "kimi", "Kimi", "Kimi API", "model-providers/kimi-mark.svg", []string{"Moonshot", "月之暗面"}, Responses, "https://platform.kimi.com/docs/api/responses", []Region{region("cn", "中国", "https://api.moonshot.cn/v1", "https://platform.moonshot.cn/console/api-keys"), region("global", "Global", "https://api.moonshot.ai/v1", "https://platform.moonshot.ai/console/api-keys")}, []string{"kimi-k3.1-pro", "kimi-k3", "kimi-k3.1-pro-highspeed"}},
	{"kimi-coding", "kimi", "Kimi", "Kimi Coding", "model-providers/kimi-mark.svg", []string{"Kimi Code", "编程套餐"}, Responses, "https://www.kimi.com/code/docs/", []Region{region("cn", "中国", "https://api.kimi.com/coding/v1", "https://www.kimi.com/code/console"), region("global", "Global", "https://api.kimi.ai/coding/v1", "https://www.kimi.com/code/console")}, []string{"k3.1-pro", "k3", "k3-256k", "kimi-for-coding", "k3.1-pro-highspeed", "kimi-for-coding-highspeed"}},
	{"aliyun", "aliyun", "阿里云百炼", "百炼 API", "model-providers/aliyun-bailian.svg", []string{"Alibaba", "Qwen", "通义千问", "Bailian"}, Responses, "https://help.aliyun.com/zh/model-studio/compatibility-with-openai-responses-api", []Region{region("cn", "中国", "https://dashscope.aliyuncs.com/compatible-mode/v1", "https://bailian.console.aliyun.com/cn-beijing/model/settings/api-key"), region("sg", "Singapore", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", "https://modelstudio.console.alibabacloud.com/?tab=dashboard#/api-key")}, []string{"qwen3.8-flash", "qwen3.8-max", "qwen3.7-max"}},
	{"aliyun-token-plan", "aliyun", "阿里云百炼", "百炼 Token Plan", "model-providers/aliyun-bailian.svg", []string{"Alibaba", "Qwen", "订阅套餐"}, Responses, "https://help.aliyun.com/zh/model-studio/token-plan-personal-quick-start", []Region{region("cn", "中国", "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "https://bailian.console.aliyun.com/cn-beijing/subscription/token-plan/personal"), region("sg", "Singapore", "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1", "https://modelstudio.console.alibabacloud.com/")}, []string{"auto", "qwen3.6-plus", "qwen3.6-flash", "kimi-k2.6", "glm-5.1", "deepseek-v4-pro", "minimax-2.7"}},
	{"volcengine", "volcengine", "火山方舟", "火山方舟 API", "model-providers/volcengine-mark.svg", []string{"豆包", "Doubao", "Ark", "火山引擎"}, Responses, "https://console.volcengine.com/ark/region:cn-beijing/docs/integrate-third-party-tools", []Region{region("cn", "北京", "https://ark.cn-beijing.volces.com/api/v3", "https://ark.volcengine.com/region:cn-beijing/apiKey")}, []string{"doubao-seed-2-1-pro-260915", "doubao-seed-2-0-lite-260428"}},
	{"volcengine-coding", "volcengine", "火山方舟", "火山 Coding Plan", "model-providers/volcengine-mark.svg", []string{"Ark", "编程套餐"}, Responses, "https://docs.volcengine.com/docs/ark/coding-plan-personal-ai-codex", []Region{region("cn", "北京", "https://ark.cn-beijing.volces.com/api/coding/v3", "https://ark.volcengine.com/region:cn-beijing/openManagement?advancedActiveKey=codingPlan")}, []string{"ark-code-latest", "doubao-seed-2.1-pro", "doubao-seed-2.1-lite", "glm-5.3", "glm-5.3-flash", "deepseek-v4-pro", "kimi-k3.1-pro"}},
	{"volcengine-agent", "volcengine", "火山方舟", "火山 Agent Plan", "model-providers/volcengine-mark.svg", []string{"Ark", "智能体套餐"}, Responses, "https://console.volcengine.com/ark/region:cn-beijing/docs/integrate-third-party-tools", []Region{region("cn", "北京", "https://ark.cn-beijing.volces.com/api/plan/v3", "https://ark.volcengine.com/region:cn-beijing/openManagement?advancedActiveKey=agentPlan")}, []string{"ark-code-latest", "doubao-seed-2.1-pro", "glm-5.3", "deepseek-v4-pro", "kimi-k3.1-pro"}},
	{"zhipu", "zhipu", "智谱 GLM", "GLM API", "model-providers/zhipu-figma.svg", []string{"BigModel", "Z.ai", "智谱"}, ChatCompletions, "https://docs.bigmodel.cn/cn/guide/develop/openai/introduction", []Region{region("cn", "中国", "https://open.bigmodel.cn/api/paas/v4", "https://bigmodel.cn/usercenter/proj-mgmt/apikeys"), region("global", "Z.ai", "https://api.z.ai/api/paas/v4", "https://z.ai/manage-apikey/apikey-list")}, []string{"glm-5.3", "glm-5.3-flash"}},
	{"zhipu-coding", "zhipu", "智谱 GLM", "GLM Coding Plan", "model-providers/zhipu-figma.svg", []string{"BigModel", "Z.ai", "编程套餐"}, ChatCompletions, "https://docs.bigmodel.cn/cn/coding-plan/quick-start", []Region{region("cn", "中国", "https://open.bigmodel.cn/api/coding/paas/v4", "https://bigmodel.cn/coding-plan/personal/overview"), region("global", "Z.ai", "https://api.z.ai/api/coding/paas/v4", "https://z.ai/manage-apikey/apikey-list")}, []string{"glm-5.3", "glm-5.3-flash"}},
	{"mimo", "mimo", "小米 MiMo", "MiMo API", "model-providers/mimo-mark.svg", []string{"Xiaomi", "小米"}, Responses, "https://mimo.mi.com/docs/en-US/api/model/list-models", []Region{region("global", "Global", "https://api.xiaomimimo.com/v1", "https://platform.xiaomimimo.com/#/console/api-keys")}, []string{"mimo-v2.6-flash", "mimo-v2.6-pro", "mimo-v2.6-pro-ultraspeed"}},
	{"mimo-plan", "mimo", "小米 MiMo", "MiMo Token Plan", "model-providers/mimo-mark.svg", []string{"Xiaomi", "小米", "订阅套餐"}, Responses, "https://mimo.mi.com/docs/en-US/tokenplan/Token%20Plan/quick-access", []Region{region("cn", "中国", "https://token-plan-cn.xiaomimimo.com/v1", "https://platform.xiaomimimo.com/#/console/plan-manage"), region("sg", "Singapore", "https://token-plan-sgp.xiaomimimo.com/v1", "https://platform.xiaomimimo.com/#/console/plan-manage"), region("eu", "Europe", "https://token-plan-ams.xiaomimimo.com/v1", "https://platform.xiaomimimo.com/#/console/plan-manage")}, []string{"mimo-v2.6-flash", "mimo-v2.6-pro", "mimo-v2.6-pro-ultraspeed"}},
	{"minimax", "minimax", "MiniMax", "MiniMax API", "model-providers/minimax-mark.svg", []string{"稀宇"}, Responses, "https://platform.minimax.cn/docs/api-reference/text-openai-api", []Region{region("cn", "中国", "https://api.minimax.cn/v1", "https://platform.minimax.cn/user-center/basic-information/interface-key"), region("global", "Global", "https://api.minimax.io/v1", "https://platform.minimax.io/user-center/basic-information/interface-key")}, []string{"MiniMax-M3.1-Flash"}},
	{"minimax-plan", "minimax", "MiniMax", "M Plan", "model-providers/minimax-mark.svg", []string{"MiniMax", "订阅套餐"}, Responses, "https://platform.minimax.cn/docs/m-plan/other-tools", []Region{region("cn", "中国", "https://api.minimax.cn/v1", "https://platform.minimax.cn/console/plan"), region("global", "Global", "https://api.minimax.io/v1", "https://platform.minimax.io/console/plan")}, []string{"MiniMax-M3.1-Flash"}},
	{"deepseek", "deepseek", "DeepSeek", "DeepSeek API", "model-providers/deepseek.svg", []string{"深度求索"}, Responses, "https://api-docs.deepseek.com/zh-cn/api/create-response/", []Region{region("global", "Global", "https://api.deepseek.com", "https://platform.deepseek.com/api_keys")}, []string{"deepseek-flash", "deepseek-v4-pro"}},
	{"custom", "custom", "自定义", "Custom API", "model-providers/customize.svg", []string{"Custom", "自定义"}, ChatCompletions, "", []Region{region("custom", "Custom", "", "")}, nil},
}

// Catalog returns an independently owned directory, never credentials.
func Catalog() []Preset {
	out := make([]Preset, 0, len(catalog))
	for _, preset := range catalog {
		out = append(out, publicPreset(preset))
	}
	return out
}

func Find(id string) (Preset, bool) {
	for _, preset := range catalog {
		if preset.ID == strings.ToLower(strings.TrimSpace(id)) {
			return publicPreset(preset), true
		}
	}
	return Preset{}, false
}

func publicPreset(p Preset) Preset {
	p.Aliases = append([]string{}, p.Aliases...)
	p.Models = append([]string{}, p.Models...)
	p.Regions = append([]Region{}, p.Regions...)
	for i := range p.Regions {
		r := &p.Regions[i]
		r.ProtocolURLs = p.protocolURLs(*r)
		r.DefaultProtocol = p.Protocol
		if r.ProtocolURLs[Responses] != "" {
			r.DefaultProtocol = Responses
		}
	}
	return p
}

func (p Preset) Region(id string) (Region, bool) {
	if id == "" && len(p.Regions) > 0 {
		return p.Regions[0], true
	}
	for _, r := range p.Regions {
		if r.ID == id {
			return r, true
		}
	}
	return Region{}, false
}

func ValidProtocol(s string) bool {
	return s == "" || s == Responses || s == ChatCompletions || s == AnthropicMessages
}
