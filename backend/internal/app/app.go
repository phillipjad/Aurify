// Package app is the application layer. It is organized around lightweight
// CQRS: writes flow through command handlers and reads through query handlers.
// See docs/adr/0003-lightweight-cqrs.md for the rationale.
package app

import (
	"github.com/phillipjad/aurify/backend/internal/app/command"
	"github.com/phillipjad/aurify/backend/internal/app/query"
)

// App is the application-layer entry point exposed to the transport layer.
// Commands mutate state (the write side); Queries read state (the read side).
// The HTTP transport depends only on this struct.
type App struct {
	Commands *command.Bus
	Queries  *query.Bus
}
