package message

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
)

type unreadHTTP func(*http.Request) (*http.Response, error)

func (f unreadHTTP) Do(r *http.Request) (*http.Response, error) { return f(r) }

type brokenOutput struct{}

func (brokenOutput) Write([]byte) (int, error) { return 0, errors.New("output closed") }

func TestReadAcknowledgesOnlySuccessfulCompleteOutput(t *testing.T) {
	for _, mode := range []string{"read", "implicit-thread", "peek", "broken"} {
		t.Run(mode, func(t *testing.T) {
			var output bytes.Buffer
			acks := 0
			requestedThread := "root"
			if mode == "implicit-thread" {
				requestedThread = ""
			}
			client := unreadHTTP(func(r *http.Request) (*http.Response, error) {
				body := `{"room_id":"room","participant_id":"pt-dev","thread_root_id":"root","messages":[{"id":"one","content":"body","sender":{"id":"user-qa","name":"Reviewer","role":"worker"}}],"ack_token":"receipt"}`
				switch r.Method {
				case http.MethodGet:
					if r.URL.Path != "/api/v1/channels/csgclaw/messages/unread" || r.URL.Query().Get("thread_root_id") != requestedThread {
						t.Fatalf("wrong read: %v", r.URL)
					}
				case http.MethodPost:
					acks++
					if !strings.Contains(output.String(), "body") {
						t.Fatal("ack happened before output")
					}
					var ack apitypes.UnreadAck
					if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
						t.Fatal(err)
					}
					if ack.Token != "receipt" || ack.RoomID != "room" || ack.ThreadRootID != "root" || ack.ParticipantID != "pt-dev" {
						t.Fatalf("ack=%+v", ack)
					}
					body = `{"acknowledged":true}`
				default:
					t.Fatal(r.Method)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})
			run := &command.Context{Program: "csgclaw", Stdout: &output, Stderr: io.Discard, HTTPClient: client}
			args := []string{"read", "--unread", "--room-id", "room", "--participant-id", "dev"}
			if requestedThread != "" {
				args = append(args, "--thread-root-id", requestedThread)
			}
			if mode == "broken" {
				run.Stdout = brokenOutput{}
			}
			if mode == "peek" {
				args = append(args, "--peek")
			}
			err := NewCmd().Run(context.Background(), run, args, command.GlobalOptions{Endpoint: "http://fixture"})
			if (err != nil) != (mode == "broken") {
				t.Fatalf("err=%v", err)
			}
			if err == nil {
				var page apitypes.UnreadPage
				if err := json.Unmarshal(output.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				want := apitypes.UnreadSender{ID: "user-qa", Name: "Reviewer", Role: "worker"}
				if len(page.Messages) != 1 || page.Messages[0].Sender != want {
					t.Fatalf("sender missing from CLI output: %s", output.String())
				}
			}
			want := 0
			if mode == "read" || mode == "implicit-thread" {
				want = 1
			}
			if acks != want {
				t.Fatalf("acks=%d want %d", acks, want)
			}
		})
	}
}

func TestCreateKeepsThreadAndRetryIdentity(t *testing.T) {
	client := unreadHTTP(func(r *http.Request) (*http.Response, error) {
		var req apitypes.CreateMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.RelatesTo == nil || req.RelatesTo.EventID != "root" || req.RelatesTo.RelType != "m.thread" || req.ClientMessageID != "stable" {
			t.Fatalf("request=%+v", req)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"sent"}`)), Header: make(http.Header)}, nil
	})
	run := &command.Context{Program: "csgclaw", Stdout: io.Discard, Stderr: io.Discard, HTTPClient: client}
	if err := NewCmd().Run(context.Background(), run, []string{"create", "--room-id", "room", "--sender-id", "dev", "--content", "Result", "--thread-root-id", "root", "--client-message-id", "stable"}, command.GlobalOptions{Endpoint: "http://fixture"}); err != nil {
		t.Fatal(err)
	}
}
