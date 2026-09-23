package api

import (
	"bytes"
	"csgclaw/internal/im"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func BenchmarkSmallChatUpload(b *testing.B) {
	for _, size := range []int{1024, 64 * 1024} {
		b.Run(fmt.Sprintf("%dKiB", size/1024), func(b *testing.B) {
			service, err := im.NewServiceFromPath(filepath.Join(b.TempDir(), "im", "state.json"))
			if err != nil {
				b.Fatal(err)
			}
			room, err := service.CreateRoom(im.CreateRoomRequest{Title: "bench", CreatorID: "user-admin"})
			if err != nil {
				b.Fatal(err)
			}
			routes := (&Handler{im: service, participantBridge: im.NewParticipantBridge("")}).Routes()
			var buffer bytes.Buffer
			form := multipart.NewWriter(&buffer)
			payload, _ := json.Marshal(map[string]string{"room_id": room.ID, "sender_id": "user-admin"})
			if err := form.WriteField("payload", string(payload)); err != nil {
				b.Fatal(err)
			}
			file, err := form.CreateFormFile("files", "small.bin")
			if err != nil {
				b.Fatal(err)
			}
			if _, err := file.Write(bytes.Repeat([]byte("x"), size)); err != nil {
				b.Fatal(err)
			}
			if err := form.Close(); err != nil {
				b.Fatal(err)
			}
			body := buffer.Bytes()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				request := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", bytes.NewReader(body))
				request.Header.Set("Content-Type", form.FormDataContentType())
				response := httptest.NewRecorder()
				routes.ServeHTTP(response, request)
				if response.Code != http.StatusCreated {
					b.Fatalf("status %d: %s", response.Code, response.Body.String())
				}
				b.StopTimer()
				if _, err := service.ClearRoomMessages(room.ID); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
			}
		})
	}
}
