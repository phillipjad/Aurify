// Package query is the read side of the application. Each read use case lives
// in its own sub-package as a single-purpose Handler, and the Bus collects them.
package query

import (
	"github.com/phillipjad/aurify/backend/internal/app/query/getcover"
	"github.com/phillipjad/aurify/backend/internal/app/query/getuser"
	"github.com/phillipjad/aurify/backend/internal/app/query/listcovers"
	"github.com/phillipjad/aurify/backend/internal/app/query/listplaylists"
)

// Bus aggregates every query handler. Add new queries as fields here.
type Bus struct {
	ListPlaylists *listplaylists.Handler
	GetCover      *getcover.Handler
	ListCovers    *listcovers.Handler
	GetUser       *getuser.Handler
}
