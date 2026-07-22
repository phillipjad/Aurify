// Package command is the write side of the application. Each use case lives in
// its own sub-package as a single-purpose Handler, and the Bus collects them so
// the transport layer has exactly one dependency to wire.
package command

import (
	"github.com/phillipjad/aurify/backend/internal/app/command/connectdsp"
	"github.com/phillipjad/aurify/backend/internal/app/command/deletecover"
	"github.com/phillipjad/aurify/backend/internal/app/command/generatecover"
)

// Bus aggregates every command handler. Add new commands as fields here.
type Bus struct {
	ConnectDSP    *connectdsp.Handler
	GenerateCover *generatecover.Handler
	DeleteCover   *deletecover.Handler
}
