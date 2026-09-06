package larkcli

import "encoding/json"

const (
	Name           = "feishu-lark-cli"
	Kind           = "lark-cli"
	SourceProvider = "feishu-participant"
)

// Payload is resolved by the Feishu-owned Source and consumed only by a
// Runtime-owned lark-cli Driver. AppSecret deliberately remains behind the
// exec provider and is never copied into this payload.
type Payload struct {
	AgentID       string `json:"agent_id"`
	ParticipantID string `json:"participant_id"`
	AppID         string `json:"app_id"`
	BaseURL       string `json:"base_url"`
	AccessToken   string `json:"access_token"`
	HelperPath    string `json:"helper_path"`
}

func Encode(payload Payload) (json.RawMessage, error) {
	return json.Marshal(payload)
}

func Decode(raw json.RawMessage) (Payload, error) {
	var payload Payload
	err := json.Unmarshal(raw, &payload)
	return payload, err
}
