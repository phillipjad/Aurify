// Package command is the write side of the application. Each use case lives in
// its own sub-package as a single-purpose Handler, and the Bus collects them so
// the transport layer has exactly one dependency to wire.
package command

import (
	"github.com/phillipjad/aurify/backend/internal/app/command/connectdsp"
	"github.com/phillipjad/aurify/backend/internal/app/command/deletecover"
	"github.com/phillipjad/aurify/backend/internal/app/command/deleterevision"
	"github.com/phillipjad/aurify/backend/internal/app/command/federatedsignin"
	"github.com/phillipjad/aurify/backend/internal/app/command/generatecover"
	"github.com/phillipjad/aurify/backend/internal/app/command/refreshsession"
	"github.com/phillipjad/aurify/backend/internal/app/command/requestpasswordreset"
	"github.com/phillipjad/aurify/backend/internal/app/command/resetpassword"
	"github.com/phillipjad/aurify/backend/internal/app/command/signin"
	"github.com/phillipjad/aurify/backend/internal/app/command/signout"
	"github.com/phillipjad/aurify/backend/internal/app/command/signup"
	"github.com/phillipjad/aurify/backend/internal/app/command/verifyemail"
)

// Bus aggregates every command handler. Add new commands as fields here.
type Bus struct {
	ConnectDSP     *connectdsp.Handler
	GenerateCover  *generatecover.Handler
	DeleteCover    *deletecover.Handler
	DeleteRevision *deleterevision.Handler

	// Authentication.
	SignUp               *signup.Handler
	SignIn               *signin.Handler
	FederatedSignIn      *federatedsignin.Handler
	RefreshSession       *refreshsession.Handler
	SignOut              *signout.Handler
	VerifyEmail          *verifyemail.Handler
	RequestPasswordReset *requestpasswordreset.Handler
	ResetPassword        *resetpassword.Handler
}
