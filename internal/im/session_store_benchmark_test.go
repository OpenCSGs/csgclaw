package im

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func BenchmarkSessionPersistence(b *testing.B) {
	for _, tc := range []struct {
		name                                  string
		rooms, messages, bodyBytes, blobEvery int
	}{
		{"one_room_1000_messages", 1, 1000, 1024, 0},
		{"twenty_rooms_100_messages", 20, 100, 256, 0},
		{"ten_rooms_50_large_bodies", 10, 50, 256, 10},
	} {
		b.Run(tc.name, func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "im", "state.json")
			state := Bootstrap{CurrentUserID: AdminUserID, Users: []User{{ID: AdminUserID, Name: "Admin"}}}
			now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
			for r := 0; r < tc.rooms; r++ {
				room := Room{ID: fmt.Sprintf("room-%d", r), Title: "Room", Members: []string{AdminUserID}}
				for m := 0; m < tc.messages; m++ {
					size := tc.bodyBytes
					if tc.blobEvery > 0 && m%tc.blobEvery == 0 {
						size = 70 * 1024
					}
					room.Messages = append(room.Messages, Message{ID: fmt.Sprintf("msg-%d", m), SenderID: AdminUserID, Content: strings.Repeat("A", size), CreatedAt: now})
				}
				state.Rooms = append(state.Rooms, room)
			}
			if err := SaveBootstrap(path, state); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state.Rooms[0].Messages[tc.messages-1].Content = fmt.Sprintf("new reply %d", i)
				if err := SaveBootstrap(path, state); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
