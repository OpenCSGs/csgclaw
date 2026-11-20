package local

import (
	"context"
	"csgclaw/internal/resourcequery"
	skillinstall "csgclaw/internal/skill/installstate"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestListSummariesMetadataIsolation(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a", "SKILL.md"), "\ufeff---\r\ndescription: >\r\n  first line\r\n  second line\r\n---\r\n"+strings.Repeat("body", 1<<19))
	mustWriteFile(t, filepath.Join(root, "b", "SKILL.md"), "---\ndescription: [broken\n---\n")
	mustWriteFile(t, filepath.Join(root, "c", "SKILL.md"), "# No frontmatter\n")
	mustWriteFile(t, filepath.Join(root, "d", "SKILL.md"), "---\ndescription: "+strings.Repeat("x", maxSkillMetadataBytes)+"\n---\n")
	mustWriteFile(t, filepath.Join(root, "a", "assets", "nested", "SKILL.md"), "---\ndescription: not a top-level skill\n---\n")
	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(outside, "SKILL.md"), "---\ndescription: outside-secret\n---\n")
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "e"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "SKILL.md"), filepath.Join(root, "e", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	items, err := ListSummaries(context.Background(), root)
	if err != nil || len(items) != 5 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if items[0].Name != "a" || items[0].Description != "first line second line" {
		t.Fatalf("description=%+v", items[0])
	}
	if items[1].Error != "invalid_metadata" || items[2].Error != "" || items[2].Description != "" || items[3].Error != "invalid_metadata" || items[4].Error != "metadata_unavailable" {
		t.Fatalf("partial failure handling=%+v", items)
	}
	if strings.Contains(fmt.Sprint(items), "outside-secret") {
		t.Fatal("outside metadata leaked")
	}
}

func TestGroupedSkillSummaries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"review", "official/review", "user/review", "user/review/examples/inner", "samples/hidden"} {
		mustWriteFile(t, filepath.Join(root, name, skillFileName), "---\ndescription: "+name+"\n---\n")
	}
	if err := os.Symlink(filepath.Join(root, "user", "review"), filepath.Join(root, "official", "linked")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	items, err := ListSummaries(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, item := range items {
		if item.Description != item.Name {
			t.Fatalf("summary read from the wrong directory: %+v", item)
		}
		names = append(names, item.Name)
	}
	want := []string{"official/review", "review", "user/review"}
	if !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}

	q := resourcequery.Query{Mode: "cursor", Size: 1, Search: "review"}
	for i, name := range want {
		page, meta, err := ListSummariesPage(ctx, root, q, func(string) bool { return true })
		if err != nil || meta.Total != len(want) || len(page) != 1 || page[0].Name != name || meta.HasMore != (i+1 < len(want)) {
			t.Fatalf("cursor page %d = %+v, meta = %+v, err = %v", i, page, meta, err)
		}
		q.Cursor = meta.NextCursor
	}
}

func TestSkillNamedAfterGroupKeepsItsOwnSummary(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "user", skillFileName), "# User")
	mustWriteFile(t, filepath.Join(root, "user", "example", skillFileName), "# Example")
	items, err := ListSummaries(context.Background(), root)
	if err != nil || len(items) != 1 || items[0].Name != "user" {
		t.Fatalf("summaries = %+v, err = %v", items, err)
	}
}

func TestSummaryReadsBoundedAndCanceled(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 300; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("skill-%03d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{}, summaryReadConcurrency)
	var calls atomic.Int32
	done := make(chan error, 1)
	go func() {
		_, err := listSummaries(ctx, root, func(ctx context.Context, _ *os.Root, name string) (SkillSummary, bool) {
			calls.Add(1)
			entered <- struct{}{}
			<-ctx.Done()
			return SkillSummary{Name: name}, true
		})
		done <- err
	}()
	for range summaryReadConcurrency {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	if calls.Load() != summaryReadConcurrency {
		t.Fatalf("unbounded reads=%d", calls.Load())
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop reader")
	}
}

func BenchmarkListSummaries(b *testing.B) {
	for _, count := range []int{300, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			root := b.TempDir()
			for i := 0; i < count; i++ {
				dir := filepath.Join(root, fmt.Sprintf("skill-%04d", i))
				if err := os.Mkdir(dir, 0700); err != nil {
					b.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\ndescription: Example skill\n---\n# Body\n"), 0600); err != nil {
					b.Fatal(err)
				}
			}
			b.ResetTimer()
			for range b.N {
				items, err := ListSummaries(context.Background(), root)
				if err != nil || len(items) != count {
					b.Fatalf("count=%d err=%v", len(items), err)
				}
			}
		})
	}
}

func TestPagedSummariesMembershipAndMetadata(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta", "zulu", "empty"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
		if name != "empty" {
			if err := os.WriteFile(filepath.Join(root, name, "SKILL.md"), []byte("---\ndescription: "+name+" description\n---\nbody"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	enabled := true
	items, meta, err := ListSummariesPage(context.Background(), root, resourcequery.Query{Mode: "page", Page: 2, Size: 1, Enabled: &enabled}, func(name string) bool { return name != "beta" })
	if err != nil || meta.Total != 2 || len(items) != 1 || items[0].Name != "zulu" || items[0].Description != "zulu description" {
		t.Fatalf("%+v %+v %v", items, meta, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := ListSummariesPage(ctx, root, resourcequery.Query{Mode: "page", Page: 1, Size: 1}, func(string) bool { return true }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestSummariesPreserveMetadataWithInvalidInstallationRecords(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "example", "SKILL.md"), "---\ndescription: example description\n---\n")
	mustWriteFile(t, filepath.Join(root, ".skillhub-lock.json"), "invalid JSON")
	for _, paged := range []bool{false, true} {
		items, err := ListSummaries(context.Background(), root)
		if paged {
			items, _, err = ListSummariesPage(context.Background(), root, resourcequery.Query{Mode: "page", Page: 1, Size: 20}, func(string) bool { return true })
		}
		if err != nil || len(items) != 1 || items[0].Name != "example" || items[0].Description != "example description" || items[0].Source != skillinstall.SourceUnknown {
			t.Fatalf("skill summaries = %+v, error = %v", items, err)
		}
	}
}

func TestSkillSummaryVersionsUseEachInstallationRecord(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"market", "local", "untracked", "builtin"} {
		mustWriteFile(t, filepath.Join(root, name, "SKILL.md"), "---\ndescription: A skill\nmetadata:\n  version: 9.9.0\n---\n")
	}
	for name, record := range map[string]skillinstall.Record{
		"market":  {Source: skillinstall.SourceMarket, Version: "v1.2.3"},
		"local":   {Source: skillinstall.SourceLocal},
		"builtin": {Source: skillinstall.SourceBuiltin, Version: strings.Repeat("a", 64)},
	} {
		if err := skillinstall.Update(root, name, &record, nil); err != nil {
			t.Fatal(err)
		}
	}
	check := func(items []SkillSummary) {
		t.Helper()
		for _, item := range items {
			want := map[string]string{"market": "v1.2.3", "local": "9.9.0", "untracked": "9.9.0", "builtin": ""}[item.Name]
			if item.Version != want {
				t.Fatalf("%s version=%q want=%q", item.Name, item.Version, want)
			}
		}
	}
	items, err := ListSummaries(context.Background(), root)
	if err != nil || len(items) != 4 {
		t.Fatalf("summaries=%+v err=%v", items, err)
	}
	check(items)
	paged, _, err := ListSummariesPage(context.Background(), root, resourcequery.Query{Mode: "page", Page: 1, Size: 10}, func(string) bool { return true })
	if err != nil || len(paged) != 4 {
		t.Fatalf("paged=%+v err=%v", paged, err)
	}
	check(paged)
}
