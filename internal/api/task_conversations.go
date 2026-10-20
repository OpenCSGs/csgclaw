package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"csgclaw/internal/im"
)

// Archive and delete share the same work guard. Neither operation stops a turn.
func (h *Handler) allowRoomManagement(w http.ResponseWriter, roomID string) bool {
	if reader, ok := h.participantWork.(roomWorkReader); ok && len(reader.ActiveWork(roomID)) > 0 {
		writeCodedAPIError(w, http.StatusConflict, "room_has_active_work", "Stop the running task before archiving or deleting this conversation.")
		return false
	}
	if h.roomTaskSvc != nil {
		release, err := h.roomTaskSvc.BeginRoomDeletion(roomID)
		if err != nil {
			writeAPIError(w, err, http.StatusConflict)
			return false
		}
		release()
	}
	return true
}

type taskConversationPage struct {
	Rooms   []taskConversationRoom `json:"rooms"`
	Total   int                    `json:"total"`
	HasMore bool                   `json:"has_more"`
}

type taskConversationRoom struct {
	im.Room
	SearchMatch *im.Message `json:"search_match,omitempty"`
}

func (room *taskConversationRoom) UnmarshalJSON(data []byte) error {
	var stored im.Room
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	var result struct {
		SearchMatch *im.Message `json:"search_match,omitempty"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	room.Room = stored
	room.SearchMatch = result.SearchMatch
	return nil
}

func conversationActivity(room im.Room) time.Time {
	latest := room.CreatedAt
	for _, message := range room.Messages {
		if message.CreatedAt.After(latest) {
			latest = message.CreatedAt
		}
	}
	return latest
}

func conversationMatches(room im.Room, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	var text strings.Builder
	text.WriteString(strings.ToLower(room.Title))
	for _, message := range room.Messages {
		text.WriteByte('\n')
		text.WriteString(strings.ToLower(message.Content))
	}
	for _, term := range terms {
		if !strings.Contains(text.String(), term) {
			return false
		}
	}
	return true
}

func conversationSearchMatch(room im.Room, terms []string) *im.Message {
	if len(terms) == 0 {
		return nil
	}
	bestIndex, bestScore := -1, 0
	for index := range room.Messages {
		content := strings.ToLower(room.Messages[index].Content)
		score := 0
		for _, term := range terms {
			if strings.Contains(content, term) {
				score++
			}
		}
		if score > bestScore || (score == bestScore && score > 0 && (bestIndex < 0 || room.Messages[index].CreatedAt.After(room.Messages[bestIndex].CreatedAt))) {
			bestIndex, bestScore = index, score
		}
	}
	if bestIndex < 0 {
		return nil
	}
	match := room.Messages[bestIndex]
	return &match
}

// Search reads the stored history before pagination, including unloaded messages.
func (h *Handler) listTaskConversations(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireLocalChannel(w)
	if !ok {
		return
	}
	if err := h.reloadIM(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	currentUserID := h.im.CurrentUserID()
	query := r.URL.Query()
	limit, offset := 20, 0
	for key, dst := range map[string]*int{"limit": &limit, "offset": &offset} {
		if value := query.Get(key); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				writeCodedAPIError(w, http.StatusBadRequest, "invalid_pagination", "Invalid pagination.")
				return
			}
			*dst = parsed
		}
	}
	if limit < 1 || limit > 100 {
		writeCodedAPIError(w, http.StatusBadRequest, "invalid_pagination", "Limit must be between 1 and 100.")
		return
	}
	archived := query.Get("archived") == "true"
	terms := strings.Fields(strings.ToLower(query.Get("q")))
	rooms := make([]im.Room, 0)
	for _, room := range h.im.ListRoomsWithOptions(im.ListMessagesOptions{IncludeThreadReplies: true}) {
		if !slices.Contains(room.Members, currentUserID) {
			continue
		}
		// Automatically provisioned agent DMs are preparation, not task history.
		if room.IsDirect && !room.TaskSession && !room.CustomTitle && !room.Pinned && !room.Archived && !hasConversationHistory(room) {
			continue
		}
		if room.Archived == archived && conversationMatches(room, terms) {
			rooms = append(rooms, room)
		}
	}
	sort.Slice(rooms, func(i, j int) bool {
		if rooms[i].Pinned != rooms[j].Pinned {
			return rooms[i].Pinned
		}
		a, b := conversationActivity(rooms[i]), conversationActivity(rooms[j])
		if !a.Equal(b) {
			return a.After(b)
		}
		return rooms[i].ID < rooms[j].ID
	})
	total := len(rooms)
	if offset > total {
		offset = total
	}
	end := offset + min(limit, total-offset)
	page := make([]taskConversationRoom, 0, end-offset)
	for _, storedRoom := range rooms[offset:end] {
		room := storedRoom
		match := conversationSearchMatch(room, terms)
		if len(room.Messages) > 1 {
			room.Messages = room.Messages[len(room.Messages)-1:]
		}
		room.Threads = nil
		page = append(page, taskConversationRoom{Room: room, SearchMatch: match})
	}
	writeJSON(w, http.StatusOK, taskConversationPage{Rooms: page, Total: total, HasMore: end < total})
}

func hasConversationHistory(room im.Room) bool {
	for _, message := range room.Messages {
		if message.Kind != "event" && message.Kind != "system" && message.Event == nil && !strings.HasPrefix(message.Content, "Bootstrap room created for admin and ") {
			return true
		}
	}
	return false
}
