package app

import "crypto/subtle"

func (s *Service) Authorize(authorization string) error {
	return s.authorize(authorization)
}

func (s *Service) authorize(authorization string) error {
	if s.cfg.API.ResolvedAdminToken == "" {
		return ErrUnauthorized
	}
	expected := "Bearer " + s.cfg.API.ResolvedAdminToken
	if subtle.ConstantTimeCompare([]byte(authorization), []byte(expected)) != 1 {
		return ErrUnauthorized
	}
	return nil
}
