package local

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const summaryReadConcurrency = 8
const maxSkillMetadataBytes = 64 * 1024

// ListSummaries reads only top-level Skill metadata. Invalid individual files
// remain visible with a path-free error; cancellation and root errors fail the read.
func ListSummaries(ctx context.Context, root string) ([]SkillSummary, error) {
	return listSummaries(ctx, root, readSkillSummary)
}

func listSummaries(ctx context.Context, root string, read func(context.Context, *os.Root, string) (SkillSummary, bool)) ([]SkillSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory, err := os.OpenRoot(root)
	if errors.Is(err, os.ErrNotExist) {
		return []SkillSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	file, err := directory.Open(".")
	if err != nil {
		return nil, err
	}
	entries, err := file.ReadDir(-1)
	_ = file.Close()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	items := make([]SkillSummary, len(names))
	present := make([]bool, len(names))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(summaryReadConcurrency, len(names)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				items[index], present[index] = read(ctx, directory, names[index])
			}
		}()
	}
enqueue:
	for index := range names {
		select {
		case jobs <- index:
		case <-ctx.Done():
			break enqueue
		}
	}
	close(jobs)
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]SkillSummary, 0, len(items))
	for index, item := range items {
		if present[index] {
			out = append(out, item)
		}
	}
	return out, nil
}

func readSkillSummary(ctx context.Context, root *os.Root, name string) (SkillSummary, bool) {
	item := SkillSummary{Name: name}
	path := filepath.Join(name, skillFileName)
	info, err := root.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return item, false
	}
	if err != nil || !info.Mode().IsRegular() {
		item.Error = "metadata_unavailable"
		return item, true
	}
	file, err := root.Open(path)
	if err != nil {
		item.Error = "metadata_unavailable"
		return item, true
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		item.Error = "metadata_unavailable"
		return item, true
	}
	reader := bufio.NewReader(io.LimitReader(file, maxSkillMetadataBytes+1))
	line, err := reader.ReadString('\n')
	if strings.TrimSpace(strings.TrimPrefix(line, "\ufeff")) != "---" {
		return item, true
	}
	var metadata strings.Builder
	bytesRead := len(line)
	for err == nil && bytesRead <= maxSkillMetadataBytes {
		if ctx.Err() != nil {
			return item, true
		}
		line, err = reader.ReadString('\n')
		bytesRead += len(line)
		if bytesRead > maxSkillMetadataBytes {
			break
		}
		if strings.TrimSpace(line) == "---" {
			item.Description, err = parseSkillDescription([]byte(metadata.String()))
			if err != nil {
				item.Error = "invalid_metadata"
			}
			return item, true
		}
		metadata.WriteString(line)
	}
	item.Error = "invalid_metadata"
	return item, true
}
