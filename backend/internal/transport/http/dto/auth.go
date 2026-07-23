package dto

// SignUpRequest registers a new email/password account.
type SignUpRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName,omitempty"`
}

// SignInRequest authenticates an email/password pair.
type SignInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TokenRequest carries a single-use token from an emailed link.
type TokenRequest struct {
	Token string `json:"token"`
}

// ForgotPasswordRequest asks for a reset link.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ResetPasswordRequest sets a new password using a reset token.
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// SessionResponse describes the signed-in user.
//
// It carries the CSRF token because the double-submit cookie is readable by
// script anyway, and returning it here saves the client from parsing
// document.cookie.
type SessionResponse struct {
	UserID        string `json:"userId"`
	Email         string `json:"email"`
	DisplayName   string `json:"displayName"`
	EmailVerified bool   `json:"emailVerified"`
	CSRFToken     string `json:"csrfToken,omitempty"`
}

// MessageResponse is a plain acknowledgement for flows that must not reveal
// whether anything actually happened, such as requesting a password reset.
type MessageResponse struct {
	Message string `json:"message"`
}
