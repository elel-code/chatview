package rpcclient

import (
	"context"

	"google.golang.org/grpc/metadata"
)

func (c *Client) ClearSession() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionToken = ""
	c.publicKey = ""
}

func (c *Client) PublicKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.publicKey
}

func (c *Client) authContext(ctx context.Context) context.Context {
	c.mu.RLock()
	token := c.sessionToken
	c.mu.RUnlock()
	if token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}
