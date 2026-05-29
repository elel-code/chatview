package core

import "context"

func (s *Service) Subscribe(ctx context.Context) (<-chan Event, <-chan error) {
	rpcEvents, rpcErrs := s.rpc.Subscribe(ctx)
	events := make(chan Event, 16)
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
				events <- Event{
					Kind:      event.Kind,
					PublicKey: event.PublicKey,
					Text:      event.Text,
					Reason:    event.Reason,
					Count:     event.Count,
				}
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
