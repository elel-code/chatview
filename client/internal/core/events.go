package core

import (
	"context"

	"chatview/client/internal/domain"
)

func (s *Service) Subscribe(ctx context.Context) (<-chan domain.Event, <-chan error) {
	rpcEvents, rpcErrs := s.rpc.Subscribe(ctx)
	events := make(chan domain.Event, 16)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		for {
			select {
			case event, ok := <-rpcEvents:
				if !ok {
					return
				}
				events <- event
			case err, ok := <-rpcErrs:
				if ok && err != nil {
					errs <- err
				}
				return
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
		}
	}()
	return events, errs
}
