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

func referenceImageTypes(images []ReferenceImage) ([]string, error) {
	var size int
	types := make([]string, 0, len(images))
	for _, image := range images {
		if len(image.Data) == 0 || len(image.Data) > MaxReferenceImageBytes-size {
			return nil, fmt.Errorf("reference images must be non-empty and total at most 32 MiB")
		}
		size += len(image.Data)
		mediaType := http.DetectContentType(image.Data)
		switch mediaType {
		case "image/jpeg", "image/png", "image/webp":
			types = append(types, mediaType)
		default:
			return nil, fmt.Errorf("reference images must be PNG, JPEG, or WebP")
		}
	}
	return types, nil
}

func referenceImageMultipart(fields map[string]string, imageField string, images []ReferenceImage) ([]byte, string, error) {
	types, err := referenceImageTypes(images)
	if err != nil {
		return nil, "", err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, "", err
		}
	}
	for i, image := range images {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": imageField, "filename": image.Name}))
		header.Set("Content-Type", types[i])
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
