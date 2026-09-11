package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/im"
	"csgclaw/internal/roomtask"
)

func TestWriteAPIErrorUsesStableRoomErrorCodes(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCode   string
		wantStatus int
	}{
		{name: "title required", err: im.ErrTitleRequired, wantCode: "title_required", wantStatus: http.StatusBadRequest},
		{name: "creator id required", err: im.ErrCreatorIDRequired, wantCode: "creator_id_required", wantStatus: http.StatusBadRequest},
		{name: "creator not found", err: im.ErrCreatorNotFound, wantCode: "creator_not_found", wantStatus: http.StatusBadRequest},
		{name: "user not found", err: fmt.Errorf("member: %w", im.ErrUserNotFound), wantCode: "user_not_found", wantStatus: http.StatusBadRequest},
		{name: "room id required", err: im.ErrRoomIDRequired, wantCode: "room_id_required", wantStatus: http.StatusBadRequest},
		{name: "room not found", err: im.ErrRoomNotFound, wantCode: "room_not_found", wantStatus: http.StatusNotFound},
		{name: "room has active tasks", err: roomtask.ErrRoomHasActiveTasks, wantCode: "room_has_active_tasks", wantStatus: http.StatusConflict},
		{name: "inviter id required", err: im.ErrInviterIDRequired, wantCode: "inviter_id_required", wantStatus: http.StatusBadRequest},
		{name: "inviter not found", err: im.ErrInviterNotFound, wantCode: "inviter_not_found", wantStatus: http.StatusBadRequest},
		{name: "inviter not room member", err: im.ErrInviterNotRoomMember, wantCode: "inviter_not_room_member", wantStatus: http.StatusBadRequest},
		{name: "user ids required", err: im.ErrUserIDsRequired, wantCode: "user_ids_required", wantStatus: http.StatusBadRequest},
		{name: "no new users", err: im.ErrNoNewUsersToInvite, wantCode: "no_new_users_to_invite", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeAPIError(rec, tt.err, http.StatusBadRequest)
			assertAPIErrorCode(t, rec, tt.wantStatus, tt.wantCode)
		})
	}
}
