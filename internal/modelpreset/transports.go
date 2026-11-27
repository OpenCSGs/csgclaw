package modelpreset

import "strings"

// A protocol URL is the base used before appending the inference resource.
func (p Preset) protocolURLs(r Region) map[string]string {
	if p.ID == "custom" {
		return nil
	}
	urls := map[string]string{p.Protocol: r.BaseURL}
	switch p.ID {
	case "openai", "kimi", "kimi-coding", "aliyun", "aliyun-token-plan",
		"volcengine", "volcengine-coding", "volcengine-agent",
		"mimo", "mimo-plan", "minimax", "minimax-plan", "deepseek":
		urls[ChatCompletions] = r.BaseURL
	}
	switch p.ID {
	case "kimi", "mimo", "mimo-plan", "minimax", "minimax-plan", "deepseek":
		urls[AnthropicMessages] = strings.TrimSuffix(r.BaseURL, "/v1") + "/anthropic/v1"
	case "kimi-coding":
		urls[AnthropicMessages] = r.BaseURL
	case "aliyun", "aliyun-token-plan":
		urls[AnthropicMessages] = strings.TrimSuffix(r.BaseURL, "/compatible-mode/v1") + "/apps/anthropic/v1"
	case "volcengine":
		urls[AnthropicMessages] = "https://ark.cn-beijing.volces.com/api/compatible/v1"
	case "volcengine-coding", "volcengine-agent":
		urls[AnthropicMessages] = strings.TrimSuffix(r.BaseURL, "/v3") + "/v1"
	case "zhipu", "zhipu-coding":
		if r.ID == "cn" {
			urls[AnthropicMessages] = "https://open.bigmodel.cn/api/anthropic/v1"
			urls[Responses] = "https://open.bigmodel.cn/api/v1"
		} else {
			urls[AnthropicMessages] = "https://api.z.ai/api/anthropic/v1"
			if p.ID == "zhipu-coding" {
				urls[Responses] = "https://api.z.ai/api/v1"
			}
		}
	}
	return urls
}

// MatchesBaseURL distinguishes built-in addresses from user-supplied gateways.
func (r Region) MatchesBaseURL(baseURL string) bool {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.EqualFold(baseURL, r.BaseURL) {
		return true
	}
	for _, candidate := range r.ProtocolURLs {
		if strings.EqualFold(baseURL, candidate) {
			return true
		}
	}
	return false
}
