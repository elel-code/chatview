package rpcclient

import (
	"context"

	eventspb "chatview/api/gen/chatview/events"
)

func (c *Client) Subscribe(ctx context.Context) (<-chan Event, <-chan error) {
	events := make(chan Event, 16)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)

		stream, err := c.events.Subscribe(c.authContext(ctx), &eventspb.SubscribeReq{ClientId: randomMessageID()})
		if err != nil {
			errs <- rpcError(err)
			return
		}
		for {
			event, err := stream.Recv()
			if err != nil {
				errs <- rpcError(err)
				return
			}
			events <- eventFromProto(event)
		}
	}()
	return events, errs
}
