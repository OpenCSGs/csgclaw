package team

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	indexFileName     = "index.json"
	teamFileName      = "team.json"
	tasksFileName     = "tasks.json"
	approvalsFileName = "approvals.json"
	presenceFileName  = "presence.json"
	eventsFileName    = "events.jsonl"
)

type Store struct {
	root string
}

type teamSnapshot struct {
	Meta      TeamMeta
	Tasks     []TeamTask
	Approvals []TeamApproval
	Presence  []MemberPresence
	Events    []TeamEvent
}

type storeIndexEntry struct {
	ID          string `json:"id"`
	Channel     string `json:"channel"`
	RoomID      string `json:"room_id"`
	Title       string `json:"title"`
	LeadAgentID string `json:"lead_agent_id"`
	Status      string `json:"status"`
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("store root is required")
	}
	return &Store{root: root}, nil
}

func (s *Store) Load() ([]teamSnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("store is required")
	}
	indexPath := filepath.Join(s.root, indexFileName)
	if _, err := os.Stat(indexPath); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var index []storeIndexEntry
	if err := readJSONFile(indexPath, &index); err != nil {
		return nil, err
	}

	out := make([]teamSnapshot, 0, len(index))
	for _, entry := range index {
		snapshot, err := s.loadTeam(entry.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	return out, nil
}

func (s *Store) Save(snapshot teamSnapshot, newEvents []TeamEvent) error {
	if s == nil {
		return fmt.Errorf("store is required")
	}
	if strings.TrimSpace(snapshot.Meta.ID) == "" {
		return fmt.Errorf("team id is required")
	}
	teamDir := s.teamDir(snapshot.Meta.ID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		return err
	}
	if err := s.appendEvents(snapshot.Meta.ID, newEvents); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(teamDir, teamFileName), snapshot.Meta); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(teamDir, tasksFileName), snapshot.Tasks); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(teamDir, approvalsFileName), snapshot.Approvals); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(teamDir, presenceFileName), snapshot.Presence); err != nil {
		return err
	}
	return s.writeIndex()
}

func (s *Store) teamDir(teamID string) string {
	return filepath.Join(s.root, teamID)
}

func (s *Store) loadTeam(teamID string) (teamSnapshot, error) {
	teamDir := s.teamDir(teamID)
	var meta TeamMeta
	if err := readJSONFile(filepath.Join(teamDir, teamFileName), &meta); err != nil {
		return teamSnapshot{}, err
	}

	var tasks []TeamTask
	if err := readOptionalJSONFile(filepath.Join(teamDir, tasksFileName), &tasks); err != nil {
		return teamSnapshot{}, err
	}
	var approvals []TeamApproval
	if err := readOptionalJSONFile(filepath.Join(teamDir, approvalsFileName), &approvals); err != nil {
		return teamSnapshot{}, err
	}
	var presence []MemberPresence
	if err := readOptionalJSONFile(filepath.Join(teamDir, presenceFileName), &presence); err != nil {
		return teamSnapshot{}, err
	}
	events, err := s.readEvents(teamID)
	if err != nil {
		return teamSnapshot{}, err
	}
	return teamSnapshot{
		Meta:      meta,
		Tasks:     tasks,
		Approvals: approvals,
		Presence:  presence,
		Events:    events,
	}, nil
}

func (s *Store) appendEvents(teamID string, events []TeamEvent) error {
	if len(events) == 0 {
		return nil
	}
	path := filepath.Join(s.teamDir(teamID), eventsFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	for _, event := range events {
		if err := enc.Encode(event); err != nil {
			return err
		}
	}
	if _, err := file.Write(buf.Bytes()); err != nil {
		return err
	}
	return file.Sync()
}

func (s *Store) readEvents(teamID string) ([]TeamEvent, error) {
	path := filepath.Join(s.teamDir(teamID), eventsFileName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var (
		events     []TeamEvent
		validBytes int64
	)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var event TeamEvent
			if unmarshalErr := json.Unmarshal(bytes.TrimSpace(line), &event); unmarshalErr != nil {
				if errors.Is(err, io.EOF) {
					if truncateErr := file.Truncate(validBytes); truncateErr != nil {
						return nil, truncateErr
					}
					if _, seekErr := file.Seek(validBytes, io.SeekStart); seekErr != nil {
						return nil, seekErr
					}
					if syncErr := file.Sync(); syncErr != nil {
						return nil, syncErr
					}
					return events, nil
				}
				return nil, fmt.Errorf("decode %s: %w", path, unmarshalErr)
			}
			events = append(events, event)
			validBytes += int64(len(line))
		}
		if errors.Is(err, io.EOF) {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (s *Store) writeIndex() error {
	index, err := s.buildIndex()
	if err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(s.root, indexFileName), index)
}

func (s *Store) buildIndex() ([]storeIndexEntry, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	index := make([]storeIndexEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		teamPath := filepath.Join(s.root, entry.Name(), teamFileName)
		if _, err := os.Stat(teamPath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		var meta TeamMeta
		if err := readJSONFile(teamPath, &meta); err != nil {
			return nil, err
		}
		index = append(index, storeIndexEntry{
			ID:          meta.ID,
			Channel:     meta.Channel,
			RoomID:      meta.RoomID,
			Title:       meta.Title,
			LeadAgentID: meta.LeadAgentID,
			Status:      meta.Status,
		})
	}
	sort.Slice(index, func(i, j int) bool { return index[i].ID < index[j].ID })
	return index, nil
}

func readOptionalJSONFile(path string, target any) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return readJSONFile(path, target)
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}

	dirHandle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer dirHandle.Close()
	return dirHandle.Sync()
}
