package transport

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	channel "csgclaw/internal/channel"
)

func TestCOTWireBudgetsPreserveDeltas(t *testing.T) {
	for _, kind := range []string{"TOOL_CALL_ARGS", "REASONING_MESSAGE_CONTENT"} {
		t.Run(kind, func(t *testing.T) {
			text := strings.Repeat("中文😀\"\\\n\t<>&\u2028", 2000)
			raw, _ := json.Marshal(map[string]string{"delta": text, "messageId": "message", "toolCallId": "tool"})
			events := []channel.COTEvent{{EventType: kind, Content: string(raw), Timestamp: 123}}
			for i := 0; i < 40; i++ {
				events = append(events, channel.COTEvent{EventType: "TOOL_CALL_END", Content: `{"toolCallId":"tool"}`, Timestamp: 124})
			}
			updates, err := SplitCOTUpdates(COTUpdateRequest{Ref: COTRef{"cot", "message"}, Events: events})
			if err != nil {
				t.Fatal(err)
			}
			var joined strings.Builder
			ends := 0
			for _, u := range updates {
				if len(u.Events) > 16 || wireSize(cotUpdateBody(u)) > 16000 {
					t.Fatal("request exceeds budget")
				}
				for _, e := range u.Events {
					if wireSize(e) > 1024 || !utf8.ValidString(e.Content) {
						t.Fatal("invalid event encoding or budget")
					}
					if e.EventType == "TOOL_CALL_END" {
						ends++
						continue
					}
					if ends > 0 || e.Timestamp != 123 {
						t.Fatal("order or timestamp changed")
					}
					var b map[string]string
					if err := json.Unmarshal([]byte(e.Content), &b); err != nil {
						t.Fatal(err)
					}
					if b["toolCallId"] != "tool" || b["messageId"] != "message" {
						t.Fatal("IDs changed")
					}
					joined.WriteString(b["delta"])
				}
			}
			if joined.String() != text || ends != 40 {
				t.Fatal("lost or duplicated content")
			}
		})
	}
}

func TestCOTWireSummaryAndMetadata(t *testing.T) {
	for kind, field := range map[string]string{"TOOL_CALL_START": "title", "TOOL_CALL_RESULT": "content"} {
		body, _ := json.Marshal(map[string]string{field: strings.Repeat("中文😀<>&", 1000), "toolCallId": "tool"})
		result, err := splitCOTEvent(channel.COTEvent{EventType: kind, Content: string(body)})
		if err != nil || len(result) != 1 {
			t.Fatalf("%v %v", result, err)
		}
		var b map[string]string
		_ = json.Unmarshal([]byte(result[0].Content), &b)
		if !strings.HasSuffix(b[field], "…") || b["toolCallId"] != "tool" || wireSize(result[0]) > 1024 {
			t.Fatal("bad summary")
		}
	}
	raw, _ := json.Marshal(map[string]string{"delta": "hello", "messageId": strings.Repeat("x", 2000)})
	updates, err := SplitCOTUpdates(COTUpdateRequest{Ref: COTRef{"cot", "message"}, Events: []channel.COTEvent{{EventType: "RUN_STARTED", Content: `{}`}, {EventType: "REASONING_MESSAGE_CONTENT", Content: string(raw)}}})
	if err == nil || updates != nil {
		t.Fatal("must reject oversized metadata before sending any batch")
	}
}

func TestCOTCompleteRequestSizeAndCount(t *testing.T) {
	events := make([]channel.COTEvent, 40)
	for i := range events {
		events[i] = channel.COTEvent{EventType: "RUN_STARTED", Content: strings.Repeat("x", 900)}
	}
	// Oversized identifiers make the complete body hit its limit before count 16.
	updates, err := SplitCOTUpdates(COTUpdateRequest{Ref: COTRef{strings.Repeat("x", 2000), "message"}, Events: events})
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, u := range updates {
		if len(u.Events) >= 16 || wireSize(cotUpdateBody(u)) > 16000 {
			t.Fatal("did not include request envelope")
		}
		total += len(u.Events)
	}
	if total != 40 {
		t.Fatal("missing events")
	}
	for _, n := range []int{1023, 1024, 1025} {
		e := channel.COTEvent{EventType: "RUN_STARTED"}
		e.Content = strings.Repeat("x", n-wireSize(e))
		err := validateCOTUpdate(COTUpdateRequest{Ref: COTRef{"cot", "message"}, Events: []channel.COTEvent{e}})
		if (err == nil) != (n <= 1024) {
			t.Fatalf("boundary %d: %v", n, err)
		}
	}
}

func TestCOTEventCountBoundary(t *testing.T) {
	for _, count := range []int{0, 1, 16, 17, 33} {
		events := make([]channel.COTEvent, count)
		for i := range events {
			events[i] = channel.COTEvent{EventType: "TOOL_CALL_END", Content: `{"toolCallId":"id"}`, Timestamp: int64(i)}
		}
		updates, err := SplitCOTUpdates(COTUpdateRequest{Ref: COTRef{"cot", "message"}, Events: events})
		if err != nil {
			t.Fatal(err)
		}
		if len(updates) != (count+15)/16 {
			t.Fatalf("%d events produced %d requests", count, len(updates))
		}
		seen := 0
		for _, u := range updates {
			for _, e := range u.Events {
				if e != events[seen] {
					t.Fatal("event order changed")
				}
				seen++
			}
		}
		if seen != count {
			t.Fatal("missing event")
		}
	}
}
