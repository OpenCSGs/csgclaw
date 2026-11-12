package im

import (
	"container/heap"
	"context"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/agentmemory"
)

// MemoryPrivateRoom checks the current room audience under the IM lock.
func (s *Service) MemoryPrivateRoom(roomID, userID, agentID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.memoryRoom(s.rooms[roomID], userID, agentID)
}

func (s *Service) memoryRoom(room *Room, userID, agentID string) bool {
	if room == nil || !room.IsDirect || room.IsOnDemand() ||
		room.CloudAgent != nil || len(room.Members) != 2 || !slices.Contains(room.Members, userID) {
		return false
	}
	if agentID == "" {
		return true
	}
	return slices.Contains(room.Members, s.resolveRoomUserIDLocked(agentID))
}

func (s *Service) MemorySourceExists(scope agentmemory.Scope, source agentmemory.Source) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	room := s.rooms[source.RoomID]
	if !s.memoryRoom(room, scope.UserID, scope.AgentID) {
		return false
	}
	index := messageIndexByID(room.Messages, source.MessageID)
	return index >= 0 && room.Messages[index].SenderID == scope.UserID
}

// MemoryHistory reads the next chronological page of persisted private user text.
// The caller has already authorized the scope as a personal local account.
func (s *Service) MemoryHistory(ctx context.Context, scope agentmemory.Scope, after, through time.Time, cursor agentmemory.HistoryCursor) ([]agentmemory.HistoryMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var page historyHeap
	for _, room := range s.rooms {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !s.memoryRoom(room, scope.UserID, scope.AgentID) {
			continue
		}
		for i := len(room.Messages) - 1; i >= 0; i-- {
			message := room.Messages[i]
			if message.SenderID != scope.UserID || message.Event != nil || message.CreatedAt.After(through) || !message.CreatedAt.After(after) ||
				strings.TrimSpace(message.Content) == "" || memoryControlText(message.Content) {
				continue
			}
			item := agentmemory.HistoryMessage{At: message.CreatedAt,
				Message: agentmemory.Message{Source: agentmemory.Source{RoomID: room.ID, MessageID: message.ID}, Text: message.Content}}
			order := item.Cursor().Compare(cursor)
			if order < 0 || (order == 0 && cursor.Offset == 0) {
				continue
			}
			if len(page) == agentmemory.HistoryPageMessages {
				if item.Cursor().Compare(page[0].Cursor()) >= 0 {
					continue
				}
				heap.Pop(&page)
			}
			heap.Push(&page, item)
		}
	}
	result := make([]agentmemory.HistoryMessage, len(page))
	for index := len(result) - 1; index >= 0; index-- {
		result[index] = heap.Pop(&page).(agentmemory.HistoryMessage)
	}
	return result, nil
}

func memoryControlText(text string) bool {
	text = strings.TrimSpace(text)
	return text == "忘掉这条" || strings.EqualFold(text, "forget this") || strings.HasPrefix(text, "<slash-command ") || strings.HasPrefix(text, "Write this down in your memory: your name is ")
}

type historyHeap []agentmemory.HistoryMessage

func (h historyHeap) Len() int { return len(h) }
func (h historyHeap) Less(i, j int) bool {
	return h[i].Cursor().Compare(h[j].Cursor()) > 0
}
func (h historyHeap) Swap(i, j int)   { h[i], h[j] = h[j], h[i] }
func (h *historyHeap) Push(value any) { *h = append(*h, value.(agentmemory.HistoryMessage)) }
func (h *historyHeap) Pop() any {
	last := len(*h) - 1
	item := (*h)[last]
	(*h)[last] = agentmemory.HistoryMessage{}
	*h = (*h)[:last]
	return item
}
