package api

import (
	"encoding/json"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/channel"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/im"
)

type diagnosticVideo struct {
	task contract.VideoGenerationTask
	turn channel.TurnContext
}

// Video jobs already persist independently of the runtime turn. Project their
// measured intervals at read time so refresh and restart retain the same total
// without extending runtime admission or maintaining another job lifecycle.
func (h *Handler) diagnosticVideos(room string) map[string][]diagnosticVideo {
	result := map[string][]diagnosticVideo{}
	messages, err := h.im.ListMessagesWithOptions(room, im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		return result
	}
	for _, message := range messages {
		if message.Metadata["video_generation"] == nil {
			continue
		}
		var video diagnosticVideo
		raw, err := json.Marshal(message.Metadata["video_generation"])
		if err != nil || json.Unmarshal(raw, &video.task) != nil {
			continue
		}
		raw, err = json.Marshal(message.Metadata["video_generation_context"])
		if err != nil || json.Unmarshal(raw, &video.turn) != nil || video.turn.RoomID != room {
			continue
		}
		result[string(video.turn.TurnID)] = append(result[string(video.turn.TurnID)], video)
	}
	return result
}

func enrichVideoDiagnostic(record *diagnostics.Snapshot, videos []diagnosticVideo, full bool) {
	running, failed := false, false
	now := time.Now()
	for _, video := range videos {
		if video.turn.AgentID != record.AgentID || video.turn.SourceMessageID != record.SourceID {
			continue
		}
		task := video.task
		start, err := time.Parse(time.RFC3339Nano, task.StartedAt)
		if err != nil {
			continue
		}
		end, endErr := time.Parse(time.RFC3339Nano, task.EndedAt)
		terminal := task.State == "completed" || task.State == "failed" || task.State == "delivery_failed"
		if terminal && (endErr != nil || end.Before(start)) {
			continue
		}
		if !terminal {
			end = now
			running = true
		}
		startMS := max(0, float64(start.Sub(record.StartedAt))/float64(time.Millisecond))
		endMS := max(startMS, float64(end.Sub(record.StartedAt))/float64(time.Millisecond))
		record.TotalMS = max(record.TotalMS, endMS)
		if terminal && task.State != "completed" {
			failed = true
			if record.Error == nil {
				record.Error = &diagnostics.Failure{Code: diagnostics.Redact(task.Error), Stage: "video.generate", Message: "Video generation or delivery failed"}
			}
		}
		if !full {
			continue
		}
		appendSpan := func(id, name, owner string, from time.Time, until time.Time, closed bool, status string, label string) {
			left := max(0, float64(from.Sub(record.StartedAt))/float64(time.Millisecond))
			right := max(left, float64(until.Sub(record.StartedAt))/float64(time.Millisecond))
			span := diagnostics.Span{ID: "video:" + task.ID + ":" + id, Name: name, Owner: owner, StartMS: left, Status: status}
			if closed {
				span.EndMS = &right
			}
			if label != "" {
				span.Details = &diagnostics.SpanDetails{Label: diagnostics.Redact(label)}
			}
			record.Spans = append(record.Spans, span)
		}
		modelStart, startErr := time.Parse(time.RFC3339Nano, task.ModelStartedAt)
		modelEnd, modelEndErr := time.Parse(time.RFC3339Nano, task.ModelCompletedAt)
		finalStatus := "running"
		if terminal {
			finalStatus = "completed"
			if task.State != "completed" {
				finalStatus = "failed"
			}
		}
		if startErr != nil || modelStart.Before(start) || modelStart.After(end) {
			// Without a measured request boundary, this interval cannot be attributed
			// to the model. Keep the total and mark the missing breakdown explicitly.
			record.Incomplete = true
			continue
		}
		appendSpan("prepare", "video.prepare", "csgclaw", start, modelStart, true, "completed", "")
		modelClosed := modelEndErr == nil && !modelEnd.Before(modelStart) && !modelEnd.After(end)
		if !modelClosed {
			modelEnd = end
		}
		modelStatus := finalStatus
		if modelClosed {
			modelStatus = "completed"
		}

		label := ""
		if task.Model != nil {
			label = task.Model.ModelID
		}
		appendSpan("model", "llm.video", "llm", modelStart, modelEnd, modelClosed || terminal, modelStatus, label)
		if modelClosed {
			appendSpan("delivery", "video.deliver", "csgclaw", modelEnd, end, terminal, finalStatus, "")
		} else if terminal && task.State == "completed" {
			record.Incomplete = true
		}

	}
	if running {
		record.Status = "running"
	} else if failed {
		record.Status = "failed"
	}
}
