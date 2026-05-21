package apiclient

import (
	"context"
	"net/http"

	"csgclaw/internal/apitypes"
)

func (c *Client) CreateNotificationBot(ctx context.Context, req apitypes.CreateBotRequest) (apitypes.Bot, error) {
	var created apitypes.Bot
	path, err := botCollectionPath(req.Channel)
	if err != nil {
		return apitypes.Bot{}, err
	}
	req.Type = "notification"
	if err := c.DoJSON(ctx, http.MethodPost, path, req, &created); err != nil {
		return apitypes.Bot{}, err
	}
	return created, nil
}

func (c *Client) PatchNotificationBot(ctx context.Context, channel, id string, req apitypes.PatchNotificationBotRequest) (apitypes.Bot, error) {
	var updated apitypes.Bot
	path, err := botItemPath(channel, id)
	if err != nil {
		return apitypes.Bot{}, err
	}
	if err := c.DoJSON(ctx, http.MethodPatch, path, req, &updated); err != nil {
		return apitypes.Bot{}, err
	}
	return updated, nil
}

func (c *Client) DeleteNotificationBot(ctx context.Context, channel, id string) error {
	return c.DeleteBot(ctx, channel, id)
}
