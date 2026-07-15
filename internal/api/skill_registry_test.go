package api

import (
	"net/http/httptest"
	"testing"
)

func TestRegistrySkillPageBoundsDoesNotReportFullLastPageAsMore(t *testing.T) {
	start, end, hasMore := registrySkillPageBounds(16, 1, 16)
	if start != 0 || end != 16 || hasMore {
		t.Fatalf("bounds = (%d,%d,%t), want (0,16,false)", start, end, hasMore)
	}
}

func TestRegistrySkillPageBoundsReportsExtraItemAsMore(t *testing.T) {
	start, end, hasMore := registrySkillPageBounds(17, 1, 16)
	if start != 0 || end != 16 || !hasMore {
		t.Fatalf("bounds = (%d,%d,%t), want (0,16,true)", start, end, hasMore)
	}
}

func TestBoundedPositiveQueryIntClampsLargeValues(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/skills/registry/search?page=999&per=999", nil)
	if got, want := boundedPositiveQueryInt(req, "page", 1, registrySkillMaxPage), registrySkillMaxPage; got != want {
		t.Fatalf("page = %d, want %d", got, want)
	}
	if got, want := boundedPositiveQueryInt(req, "per", registrySkillPageSize, registrySkillMaxPageSize), registrySkillMaxPageSize; got != want {
		t.Fatalf("per = %d, want %d", got, want)
	}
}
