package modelprovider

import (
	"bytes"
	"context"
	"csgclaw/internal/modelcap"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if len(progress) != 4 || progress[0].Status != "submitting" || progress[1].ID != "video_1" || progress[1].Status != "queued" || progress[2].Status != "completed" || progress[3].Status != "downloading" {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestGenerateVideoAcceptsChunkedContentWithoutContentLength(t *testing.T) {
	previous := videoPollInterval
	videoPollInterval = time.Millisecond
	t.Cleanup(func() { videoPollInterval = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "video_1", "status": "completed"})
		case r.URL.Path == "/videos/video_1/content":
			w.Header().Set("Content-Type", "video/mp4")
			flusher := w.(http.Flusher)
			_, _ = w.Write([]byte("video-"))
			flusher.Flush()
			_, _ = w.Write([]byte("bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result, err := GenerateVideo(context.Background(), server.Client(), server.URL, "", nil, "model", "prompt", VideoGenerationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Content.Close()
	data := new(bytes.Buffer)
	_, _ = data.ReadFrom(result.Content)
	if data.String() != "video-bytes" || result.SizeBytes != int64(len("video-bytes")) {
		t.Fatalf("content = %q, size = %d", data.String(), result.SizeBytes)
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

func TestGenerateVideoUploadsFirstFrame(t *testing.T) {
	image := []byte("\x89PNG\r\n\x1a\nfixture")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse video request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer r.MultipartForm.RemoveAll()
			if r.Method != http.MethodPost || r.FormValue("model") != "seedance-2.0-mini" ||
				r.FormValue("prompt") != "让海浪动起来" || r.FormValue("size") != "768P" || r.FormValue("seconds") != "6" ||
				r.Header.Get("Idempotency-Key") != "call-video" {
				t.Errorf("unexpected video request: %s %v", r.Method, r.MultipartForm.Value)
			}
			file, header, err := r.FormFile("input_reference")
			if err != nil {
				t.Errorf("missing first frame: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var data bytes.Buffer
			_, err = data.ReadFrom(file)
			_ = file.Close()
			if err != nil || !bytes.Equal(data.Bytes(), image) || header.Filename != "first-frame.png" || header.Header.Get("Content-Type") != "image/png" {
				t.Errorf("unexpected first frame: %v %v", header, err)
			}
			_, _ = w.Write([]byte(`{"id":"video_1","status":"completed"}`))
		case "/v1/videos/video_1/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	result, err := GenerateVideo(context.Background(), server.Client(), server.URL+"/v1", "", nil, "seedance-2.0-mini", "让海浪动起来", VideoGenerationOptions{
		Size: "768P", Seconds: 6, RequestID: "call-video", Reference: &ReferenceImage{Name: "first-frame.png", Data: image},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Content.Close()
	if result.MediaType != "video/mp4" {
		t.Fatalf("video media type = %q", result.MediaType)
	}
}

func TestGenerateVideoFallsBackToJSONAfterMultipartRejection(t *testing.T) {
	frame := []byte("\x89PNG\r\n\x1a\nfixture")
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/videos/video_1/content" {
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
			return
		}
		posts++
		if r.Method != http.MethodPost || r.URL.Path != "/videos" || r.Header.Get("Idempotency-Key") != "same-call" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if posts == 1 {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
				t.Error("first request must use multipart")
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"selected model does not support multipart input_reference"}}`))
			return
		}
		var request struct {
			Model     string
			Prompt    string
			Size      string
			Seconds   int
			Reference struct {
				ImageURL string `json:"image_url"`
			} `json:"input_reference"`
		}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil || r.Header.Get("Content-Type") != "application/json" ||
			request.Model != "seedance-2.0-mini" || request.Prompt != "云缓慢流动" ||
			request.Size != "480P" || request.Seconds != 2 ||
			request.Reference.ImageURL != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(frame) {
			t.Errorf("unexpected JSON reference request: %+v, %v", request, err)
		}
		_, _ = w.Write([]byte(`{"id":"video_1","status":"completed"}`))
	}))
	defer server.Close()
	result, err := GenerateVideo(context.Background(), server.Client(), server.URL, "", nil, "seedance-2.0-mini", "云缓慢流动", VideoGenerationOptions{
		Size: "480P", Seconds: 2, RequestID: "same-call", Reference: &ReferenceImage{Name: "frame.png", Data: frame},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Content.Close()
	if posts != 2 || result.MediaType != "video/mp4" {
		t.Fatalf("posts = %d, result = %+v", posts, result)
	}
}
