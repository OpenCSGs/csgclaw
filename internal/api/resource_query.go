package api

import (
	"csgclaw/internal/resourcequery"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type resourcePage[T any] struct {
	Items      []T    `json:"items"`
	Page       int    `json:"page,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Total      int    `json:"total"`
	Revision   string `json:"list_revision"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

func newResourcePage[T any](items []T, meta resourcequery.Metadata) resourcePage[T] {
	return resourcePage[T]{
		Items: items, Page: meta.Page, Limit: meta.Per, Total: meta.Total,
		Revision: meta.Revision, NextCursor: meta.NextCursor, HasMore: meta.HasMore,
	}
}

func parseResourceQuery(r *http.Request) (*resourcequery.Query, error) {
	values := r.URL.Query()
	mode := values.Get("pagination")
	if mode == "" {
		return nil, nil
	}
	q := &resourcequery.Query{Mode: mode, Page: 1, Size: 20, Search: strings.TrimSpace(values.Get("search")), Cursor: values.Get("cursor"), Revision: values.Get("list_revision"), Exclude: values["exclude"]}
	if values.Has("per") {
		return nil, resourcequery.ErrInvalid
	}
	if mode == "cursor" {
		if values.Has("page") {
			return nil, resourcequery.ErrInvalid
		}
	} else if mode != "page" || values.Has("cursor") {
		return nil, resourcequery.ErrInvalid
	}
	for key, target := range map[string]*int{"page": &q.Page, "limit": &q.Size} {
		if values.Has(key) {
			n, err := strconv.Atoi(values.Get(key))
			if err != nil || n < 1 {
				return nil, resourcequery.ErrInvalid
			}
			*target = n
		}
	}
	if q.Size > 100 || len(q.Cursor) > 4096 || len(q.Search) > 256 {
		return nil, resourcequery.ErrInvalid
	}
	if values.Has("enabled") {
		enabled, err := strconv.ParseBool(values.Get("enabled"))
		if err != nil {
			return nil, resourcequery.ErrInvalid
		}
		q.Enabled = &enabled
	}
	return q, nil
}

func writeResourceQueryError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, resourcequery.ErrChanged) {
		writeCodedAPIError(w, http.StatusConflict, "resource_list_changed", err.Error())
		return true
	}
	if errors.Is(err, resourcequery.ErrInvalid) {
		writeCodedAPIError(w, http.StatusBadRequest, "invalid_resource_query", err.Error())
		return true
	}
	return false
}
