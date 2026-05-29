package ui

import (
	"context"
	"fmt"
	"time"

	"chatview/client/internal/domain"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (a *Application) refreshOutboxStatus() {
	if a.outboxLabel == nil {
		return
	}
	status := a.service.OutboxStatus()
	a.lastOutbox = status
	a.setOutboxStatus(status)
}

func (a *Application) setOutboxStatus(status domain.OutboxStatus) {
	if a.outboxLabel == nil {
		return
	}
	if status.Pending == 0 && status.Failed == 0 {
		a.outboxLabel.SetText("Outbox clear")
		a.outboxLabel.Importance = widget.SuccessImportance
		if a.retryOutbox != nil {
			a.retryOutbox.Disable()
		}
		if a.clearOutbox != nil {
			a.clearOutbox.Disable()
		}
		return
	}
	a.outboxLabel.SetText(fmt.Sprintf("pending %d  failed %d", status.Pending, status.Failed))
	if status.Failed > 0 {
		a.outboxLabel.Importance = widget.WarningImportance
	} else {
		a.outboxLabel.Importance = widget.LowImportance
	}
	if a.retryOutbox != nil {
		if status.Failed > 0 && !a.offline {
			a.retryOutbox.Enable()
		} else {
			a.retryOutbox.Disable()
		}
	}
	if a.clearOutbox != nil {
		if status.Failed > 0 {
			a.clearOutbox.Enable()
		} else {
			a.clearOutbox.Disable()
		}
	}
}

func (a *Application) startOutboxPoller() {
	a.stopOutboxPoller()
	ctx, cancel := context.WithCancel(a.sessionContext())
	a.outboxPollCancel = cancel
	version := a.sessionVersion
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status := a.service.OutboxStatus()
				fyne.Do(func() {
					if ctx.Err() != nil || !a.isCurrentSession(version) {
						return
					}
					previous := a.lastOutbox
					if previous == status {
						return
					}
					a.lastOutbox = status
					a.setOutboxStatus(status)
					if a.selectedFriend != nil && (status.Pending < previous.Pending || status.Failed != previous.Failed) {
						a.loadHistory(a.selectedFriend.PublicKey)
					}
				})
			}
		}
	}()
}

func (a *Application) stopOutboxPoller() {
	if a.outboxPollCancel != nil {
		a.outboxPollCancel()
		a.outboxPollCancel = nil
	}
}

func (a *Application) retryFailedOutbox() {
	if a.offline {
		a.setStatus("offline")
		return
	}
	if a.retryOutbox != nil {
		a.retryOutbox.Disable()
	}
	a.setStatus("retrying failed messages...")
	go func() {
		err := a.service.RetryFailedOutbox()
		fyne.Do(func() {
			if err != nil {
				a.setStatus(err.Error())
				a.notify("Retry failed", err.Error(), widget.DangerImportance)
				a.refreshOutboxStatus()
				return
			}
			a.refreshOutboxStatus()
			if a.selectedFriend != nil {
				a.loadHistory(a.selectedFriend.PublicKey)
			}
			a.setStatus("failed messages queued")
			a.notify("Retry queued", "Failed messages will be sent again", widget.SuccessImportance)
		})
	}()
}

func (a *Application) clearFailedOutbox() {
	confirm := dialog.NewConfirm("Clear Failed", "Remove failed outbox messages from the local cache?", func(ok bool) {
		if !ok {
			return
		}
		if a.clearOutbox != nil {
			a.clearOutbox.Disable()
		}
		a.setStatus("clearing failed messages...")
		go func() {
			err := a.service.ClearFailedOutbox()
			fyne.Do(func() {
				if err != nil {
					a.setStatus(err.Error())
					a.notify("Clear failed", err.Error(), widget.DangerImportance)
					a.refreshOutboxStatus()
					return
				}
				a.refreshOutboxStatus()
				if a.selectedFriend != nil {
					a.loadHistory(a.selectedFriend.PublicKey)
				}
				a.setStatus("failed messages cleared")
				a.notify("Outbox cleared", "Failed messages removed", widget.SuccessImportance)
			})
		}()
	}, a.window)
	confirm.SetConfirmImportance(widget.DangerImportance)
	confirm.Show()
}
