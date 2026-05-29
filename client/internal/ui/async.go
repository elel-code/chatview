package ui

import (
	"context"

	"fyne.io/fyne/v2"
)

type sessionSnapshot struct {
	ctx     context.Context
	version int
}

func (a *Application) currentSessionSnapshot() sessionSnapshot {
	return sessionSnapshot{
		ctx:     a.sessionContext(),
		version: a.sessionVersion,
	}
}

func (a *Application) isActiveSnapshot(snapshot sessionSnapshot) bool {
	return snapshot.ctx.Err() == nil && a.isCurrentSession(snapshot.version)
}

func (a *Application) doInSession(snapshot sessionSnapshot, fn func()) {
	fyne.Do(func() {
		if a.isActiveSnapshot(snapshot) {
			fn()
		}
	})
}

func runSessionTask[T any](a *Application, work func(context.Context) (T, error), apply func(T, error)) {
	snapshot := a.currentSessionSnapshot()
	go func() {
		result, err := work(snapshot.ctx)
		a.doInSession(snapshot, func() {
			apply(result, err)
		})
	}()
}
