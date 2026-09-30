package presentation

import (
	"fmt"
	"strings"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
)

func button(label, operation string, value map[string]any) map[string]any {
	if value == nil {
		value = map[string]any{}
	}
	value["operation"] = operation
	return map[string]any{"tag": "button", "text": map[string]any{"tag": "plain_text", "content": label}, "type": "primary", "behaviors": []any{map[string]any{"type": "callback", "value": value}}}
}

func InteractionCard(req agentengine.InteractionRequest) (map[string]any, error) {
	elements := []any{markdownElement(req.Title)}
	switch snapshot := req.Payload.(type) {
	case activity.ActivitySnapshot:
		if snapshot.Status != activity.ActionStatusPending {
			return Card(req.Title + "\n\n" + interactionStatus(string(snapshot.Status))), nil
		}
		if !snapshot.ExpiresAt.IsZero() {
			elements = append(elements, markdownElement("有效时间至 "+snapshot.ExpiresAt.Local().Format("15:04:05")))
		}
		for _, option := range snapshot.Options {
			elements = append(elements, button(option.Label, "resolve", map[string]any{"option_id": option.ID}))
		}
	case activity.UserInputSnapshot:
		if snapshot.Status != activity.UserInputStatusPending {
			return Card(req.Title + "\n\n" + interactionStatus(string(snapshot.Status))), nil
		}
		fields := []any{}
		for index, q := range snapshot.Questions {
			fields = append(fields, markdownElement(q.Question))
			if len(q.Options) > 0 {
				options := []any{}
				for i, o := range q.Options {
					options = append(options, map[string]any{"text": map[string]any{"tag": "plain_text", "content": o.Label}, "value": fmt.Sprint(i)})
				}
				fields = append(fields, map[string]any{"tag": "select_static", "name": fmt.Sprintf("choice_%d", index), "options": options, "placeholder": map[string]any{"tag": "plain_text", "content": "请选择"}})
			}
			if q.IsOther || len(q.Options) == 0 {
				fields = append(fields, map[string]any{"tag": "input", "name": fmt.Sprintf("answer_%d", index), "placeholder": map[string]any{"tag": "plain_text", "content": "输入回答"}})
			}
		}
		submit := button("提交", "resolve", nil)
		submit["name"] = "submit"
		submit["form_action_type"] = "submit"
		fields = append(fields, submit)
		elements = append(elements, map[string]any{"tag": "form", "name": "answers", "elements": fields})
	default:
		return nil, fmt.Errorf("unsupported interaction payload")
	}
	if !req.Detached {
		elements = append(elements, button("取消本次执行", "cancel", nil))
	}
	return map[string]any{"schema": "2.0", "config": map[string]any{"update_multi": true}, "body": map[string]any{"elements": elements}}, nil
}

func InteractionAnswers(req agentengine.InteractionRequest, form map[string]any) (map[string]agentengine.InteractionAnswer, error) {
	snapshot, ok := req.Payload.(activity.UserInputSnapshot)
	if !ok {
		return nil, fmt.Errorf("invalid question payload")
	}
	answers := make(map[string]agentengine.InteractionAnswer, len(snapshot.Questions))
	for index, q := range snapshot.Questions {
		values := []string{}
		choice, _ := form[fmt.Sprintf("choice_%d", index)].(string)
		if choice != "" {
			found := false
			for i, o := range q.Options {
				if choice == fmt.Sprint(i) {
					values = append(values, o.Label)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("invalid answer option")
			}
		}
		text, _ := form[fmt.Sprintf("answer_%d", index)].(string)
		if strings.TrimSpace(text) != "" && (q.IsOther || len(q.Options) == 0) {
			values = append(values, "user_note: "+text)
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("请回答：%s", q.Question)
		}
		answers[q.ID] = agentengine.InteractionAnswer{Values: values}
	}
	return answers, nil
}

func interactionStatus(status string) string {
	switch status {
	case "allowed":
		return "已允许"
	case "rejected":
		return "已拒绝"
	case "answered":
		return "已提交"
	case "skipped":
		return "已跳过"
	case "expired":
		return "已过期"
	case "canceled", "cancelled":
		return "已取消"
	default:
		return "已结束"
	}
}
