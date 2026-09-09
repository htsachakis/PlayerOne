package main

import "strings"

// maxClientErrorLength bounds what the frontend can write into the log, so a
// runaway loop cannot fill the disk.
const maxClientErrorLength = 4000

// LogClientError records a problem that happened in the interface.
//
// Without this, a JavaScript exception in a production build vanishes: there is
// no console to read and no devtools to open. Sending them to the same log as
// the backend is what makes a bug report from a user actionable.
func (a *App) LogClientError(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	if len(message) > maxClientErrorLength {
		message = message[:maxClientErrorLength] + "… (truncated)"
	}

	// Newlines are collapsed so one stack trace stays one log record.
	a.log.Error("ui: %s", strings.ReplaceAll(message, "\n", " | "))
}

// LogClientInfo records an interface milestone, used for tracing startup.
func (a *App) LogClientInfo(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	if len(message) > maxClientErrorLength {
		message = message[:maxClientErrorLength] + "… (truncated)"
	}
	a.log.Debug("ui: %s", strings.ReplaceAll(message, "\n", " | "))
}
