package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
