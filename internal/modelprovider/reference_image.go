package modelprovider

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
)

const (
	MaxReferenceImageBytes = 32 << 20
	MaxImageEditReferences = 16
)

// ReferenceImage contains an uploaded image, without a local path or remote URL.
type ReferenceImage struct {
	Name string
	Data []byte
}

func referenceImageMultipart(fields map[string]string, imageField string, images []ReferenceImage) ([]byte, string, error) {
	var size int
	for _, image := range images {
		if len(image.Data) == 0 || len(image.Data) > MaxReferenceImageBytes-size {
			return nil, "", fmt.Errorf("reference images must be non-empty and total at most 32 MiB")
		}
		size += len(image.Data)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, "", err
		}
	}
	for _, image := range images {
		mediaType := http.DetectContentType(image.Data)
		switch mediaType {
		case "image/jpeg", "image/png", "image/webp":
		default:
			return nil, "", fmt.Errorf("reference images must be PNG, JPEG, or WebP")
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": imageField, "filename": image.Name}))
		header.Set("Content-Type", mediaType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(image.Data); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}
