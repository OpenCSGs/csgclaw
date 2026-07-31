package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteHubPageOptions(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr string
		page    int
		per     int
	}{
		{name: "defaults", page: 1, per: 12},
		{name: "explicit values", query: "?page=2&per=24", page: 2, per: 24},
		{name: "rejects invalid page", query: "?page=0&per=12", wantErr: "page must be a positive integer"},
		{name: "rejects per above limit", query: "?page=1&per=101", wantErr: "per must be an integer between 1 and 100"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/resource"+test.query, nil)
			page, per, err := remoteHubPageOptions(req, 1, 12, 100)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("remoteHubPageOptions() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("remoteHubPageOptions() error = %v", err)
			}
			if page != test.page || per != test.per {
				t.Fatalf("remoteHubPageOptions() = (%d, %d), want (%d, %d)", page, per, test.page, test.per)
			}
		})
	}
}
