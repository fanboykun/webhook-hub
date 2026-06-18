package ingress

import "errors"

var (
	ErrUnknownIntegration = errors.New("unknown integration")
	ErrWrongSource        = errors.New("integration source mismatch")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrMalformedPayload   = errors.New("malformed payload")
)
