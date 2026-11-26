package agentsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

var ErrMessageConflict = errors.New("message ID was already used with different input or authorization")

type messageReceipt struct {
	Fingerprint   string   `json:"fingerprint"`
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
}

// PrepareMessage preserves attachment identities across retries, including when
// the server exits after preparing the input and before starting the native turn.
func (s *Store) PrepareMessage(ctx context.Context, responseID, fingerprint string, prepare func() ([]string, error)) ([]string, error) {
	sum := sha256.Sum256([]byte(responseID))
	dir := filepath.Join(s.dir, "messages")
	path := filepath.Join(dir, hex.EncodeToString(sum[:]))
	release, err := s.messages.Acquire(ctx, path)
	if err != nil {
		return nil, err
	}
	defer release()
	data, err := os.ReadFile(path)
	if err == nil {
		var receipt messageReceipt
		if err := json.Unmarshal(data, &receipt); err != nil {
			return nil, err
		}
		if receipt.Fingerprint != fingerprint {
			return nil, ErrMessageConflict
		}
		return receipt.AttachmentIDs, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ids, err := prepare()
	if err != nil {
		return nil, err
	}
	data, err = json.Marshal(messageReceipt{Fingerprint: fingerprint, AttachmentIDs: ids})
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, ".input-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	return ids, os.Rename(f.Name(), path)
}
