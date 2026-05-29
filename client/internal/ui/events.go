package ui

import (
	"context"
	"strings"
	"time"

	"chatview/client/internal/core"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func (a *Application) watchEvents() {
	a.stopEvents()
	ctx, cancel := context.WithCancel(a.sessionContext())
	a.eventCancel = cancel
	version := a.sessionVersion

	go func() {
		backoff := time.Second
		for {
			if ctx.Err() != nil {
				return
			}
			events, errs := a.service.Subscribe(ctx)
			var streamErr error
		stream:
			for events != nil || errs != nil {
				select {
				case event, ok := <-events:
					if !ok {
						events = nil
						continue
					}
					backoff = time.Second
					a.handleEvent(event)
				case err, ok := <-errs:
					if !ok {
						errs = nil
						continue
					}
					if err == nil {
						continue
					}
					if ctx.Err() != nil {
						return
					}
					streamErr = err
					break stream
				case <-ctx.Done():
					return
				}
			}
			if ctx.Err() != nil {
				return
			}
			fyne.Do(func() {
				if !a.isCurrentSession(version) {
					return
				}
				if streamErr != nil {
					a.setStatus("event stream reconnecting: " + streamErr.Error())
				} else {
					a.setStatus("event stream reconnecting")
				}
			})
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff = min(backoff*2, 30*time.Second)
		}
	}()
}

func (a *Application) stopEvents() {
	if a.eventCancel != nil {
		a.eventCancel()
		a.eventCancel = nil
	}
}

func (a *Application) handleEvent(event core.Event) {
	fyne.Do(func() {
		switch event.Kind {
		case "new_message":
			body := shortKey(event.PublicKey)
			if strings.TrimSpace(event.Text) != "" {
				body += ": " + event.Text
			}
			a.notifyWithAction("New message", body, widget.HighImportance, func() {
				a.selectConversation(event.PublicKey)
			})
			a.refreshFriends()
			a.refreshOutboxStatus()
			a.syncConversation(event.PublicKey, event.Count)
		case "friend_status", "admin_update":
			a.refreshFriends()
			a.refreshAdmin()
		case "system_broadcast":
			a.notify("Broadcast", event.Text, widget.WarningImportance)
		case "force_offline":
			reason := strings.TrimSpace(event.Reason)
			if reason == "" {
				reason = "Session forced offline"
			}
			a.stopEvents()
			a.service.Logout()
			a.resetSessionState()
			a.showAuth()
			a.setStatus("force offline: " + reason)
			a.notify("Forced offline", reason, widget.DangerImportance)
		}
	})
}
