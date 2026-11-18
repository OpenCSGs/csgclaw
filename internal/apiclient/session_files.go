package apiclient

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"

	"csgclaw/internal/apitypes"
)

func (c *Client) UploadSessionFile(ctx context.Context, path, name string, source *os.File) ([]apitypes.MessageAttachment, error) {
	reader, writer := io.Pipe()
	defer reader.Close()
	form := multipart.NewWriter(writer)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", form.FormDataContentType())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		part, err := form.CreateFormFile("files", name)
		if err == nil {
			_, err = io.Copy(part, source)
		}
		if err == nil {
			err = form.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	defer func() { reader.Close(); <-finished }()
	var out struct {
		Attachments []apitypes.MessageAttachment `json:"attachments"`
	}
	err = c.doJSONRequest(req, &out)
	return out.Attachments, err
}

func (c *Client) OpenPublishedAttachment(ctx context.Context, id string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/api/v1/attachments/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, ExtractAPIError(resp)
	}
	return resp.Body, nil
}
