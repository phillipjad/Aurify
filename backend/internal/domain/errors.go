package domain

import "errors"

// Sentinel errors returned by the domain and application layers. The transport
// layer maps these to HTTP status codes.
var (
	ErrNotFound            = errors.New("resource not found")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrUnsupportedPlatform = errors.New("unsupported dsp platform")
)
