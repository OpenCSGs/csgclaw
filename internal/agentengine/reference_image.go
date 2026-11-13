package agentengine

import (
	"context"
	"fmt"
	"io"
	"strings"

	"csgclaw/internal/modelprovider"
)

// Reference indices match the order of attached images in the runtime input.
func loadReferenceImages(ctx context.Context, input []InputPart, indices []int, maxCount int) ([]modelprovider.ReferenceImage, []string, error) {
	if len(indices) == 0 {
		return nil, nil, nil
	}
	if len(indices) > maxCount {
		return nil, nil, fmt.Errorf("this tool supports up to %d reference images", maxCount)
	}
	var images []*InputFile
	for _, part := range input {
		if part.File != nil && part.File.Resolved != nil && strings.HasPrefix(part.File.Resolved.MediaType, "image/") {
			images = append(images, part.File)
		}
	}

	var references []modelprovider.ReferenceImage
	var attachmentIDs []string
	seen := make(map[int]bool, len(indices))
	remaining := int64(modelprovider.MaxReferenceImageBytes)
	for _, index := range indices {
		if index < 1 || index > len(images) || seen[index] {
			return nil, nil, fmt.Errorf("reference image %d must identify a distinct image attached to this turn", index)
		}
		seen[index] = true
		file := images[index-1]
		if file.Resolved.SizeBytes <= 0 || file.Resolved.SizeBytes > remaining {
			return nil, nil, fmt.Errorf("reference images must be non-empty and total at most 32 MiB")
		}
		reader, err := file.Resolved.Open(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("reference image %d is unavailable", index)
		}
		data, err := io.ReadAll(io.LimitReader(reader, file.Resolved.SizeBytes+1))
		_ = reader.Close()
		if err != nil || int64(len(data)) != file.Resolved.SizeBytes {
			return nil, nil, fmt.Errorf("reference image %d could not be read", index)
		}
		remaining -= int64(len(data))
		references = append(references, modelprovider.ReferenceImage{Name: file.Resolved.Name, Data: data})
		attachmentIDs = append(attachmentIDs, file.AttachmentID)
	}
	return references, attachmentIDs, nil
}
