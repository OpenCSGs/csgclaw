package modelprovider

import (
	"bytes"
	"context"
	"csgclaw/internal/modelcap"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveVideoGenerationOptionsUsesAdvertisedDefaults(t *testing.T) {
	got, err := ResolveVideoGenerationOptions(VideoGenerationOptions{}, modelcap.VideoGeneration{Sizes: []string{"768P", "1080P"}, Seconds: []int{6, 10}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Size != "768P" || got.Seconds != 6 {
		t.Fatalf("options = %#v, want provider defaults", got)
	}
}

func TestResolveVideoGenerationOptionsRejectsUnsupportedExplicitValues(t *testing.T) {
	capabilities := modelcap.VideoGeneration{Sizes: []string{"768P", "1080P"}, Seconds: []int{6, 10}}
	if _, err := ResolveVideoGenerationOptions(VideoGenerationOptions{Size: "1280x720"}, capabilities); err == nil {
		t.Fatal("unsupported size error = nil")
	}
	if _, err := ResolveVideoGenerationOptions(VideoGenerationOptions{Seconds: 2}, capabilities); err == nil {
		t.Fatal("unsupported seconds error = nil")
	}
}

func TestGenerateVideoUsesAIGatewayAsyncAPI(t *testing.T) {
	previous := videoPollInterval
	videoPollInterval = time.Millisecond
	t.Cleanup(func() { videoPollInterval = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/videos":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["model"] != "MiniMax-Hailuo-2.3" || body["prompt"] != "ocean" || body["size"] != "1280x720" || body["seconds"] != float64(6) {
				t.Fatalf("request = %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "video_1", "status": "queued"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/video_1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "video_1", "status": "completed"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/video_1/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	var progress []VideoGenerationProgress
	got, err := GenerateVideo(context.Background(), server.Client(), server.URL+"/v1", "secret", nil, "MiniMax-Hailuo-2.3", "ocean", VideoGenerationOptions{Size: "1280x720", Seconds: 6, Progress: func(update VideoGenerationProgress) { progress = append(progress, update) }})
	if err != nil {
		t.Fatal(err)
	}
	defer got.Content.Close()
	data := new(bytes.Buffer)
	_, _ = data.ReadFrom(got.Content)
	if data.String() != "video-bytes" || got.SizeBytes != int64(len("video-bytes")) || got.MediaType != "video/mp4" {
		t.Fatalf("result = %#v", got)
	}
	if len(progress) != 3 || progress[0].ID != "video_1" || progress[0].Status != "queued" || progress[1].Status != "completed" || progress[2].Status != "downloading" {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestGenerateVideoRetriesPollingWithoutCreatingAnotherJob(t *testing.T) {
	previousPoll, previousRetry := videoPollInterval, videoRetryBaseDelay
	videoPollInterval, videoRetryBaseDelay = time.Millisecond, time.Millisecond
	t.Cleanup(func() { videoPollInterval, videoRetryBaseDelay = previousPoll, previousRetry })
	posts, polls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			posts++
			if r.Header.Get("Idempotency-Key") != "tool-call-1" {
				t.Fatalf("idempotency key = %q", r.Header.Get("Idempotency-Key"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "video_1", "status": "queued"})
		case r.URL.Path == "/videos/video_1":
			polls++
			if polls == 1 {
				http.Error(w, "temporary", http.StatusBadGateway)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed"})
		case r.URL.Path == "/videos/video_1/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		}
	}))
	defer server.Close()
	result, err := GenerateVideo(context.Background(), server.Client(), server.URL, "", nil, "model", "prompt", VideoGenerationOptions{RequestID: "tool-call-1"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Content.Close()
	if posts != 1 || polls != 2 {
		t.Fatalf("posts=%d polls=%d, want 1 and 2", posts, polls)
	}
}

func TestGenerateVideoResumesExistingJobWithoutPost(t *testing.T) {
	previous := videoPollInterval
	videoPollInterval = time.Millisecond
	t.Cleanup(func() { videoPollInterval = previous })
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			t.Fatalf("resume unexpectedly created a new job")
		}
		switch r.URL.Path {
		case "/videos/existing":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed"})
		case "/videos/existing/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result, err := GenerateVideo(context.Background(), server.Client(), server.URL, "", nil, "model", "prompt", VideoGenerationOptions{ResumeID: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Content.Close()
	if posts != 0 {
		t.Fatalf("posts=%d, want 0", posts)
	}
}
