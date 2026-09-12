package auth

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/utils/connerr"
)

// Rate limiting policy.
//
// Authentication endpoints fail CLOSED. When the limiter cannot answer, whether
// because the cache backend is unreachable or because no cache was wired at
// all, the request is refused with Unavailable rather than allowed through.
//
// The reasoning is that the moment the limiter is unavailable is exactly the
// moment an attacker wants it gone: a limiter that disappears under load or
// under a targeted outage is not a control, it is a formality. Login,
// registration, password reset, TOTP verification and device polling are all
// brute-forceable, all bounded by a small secret, and all recoverable from by
// retrying a few seconds later. Refusing them during a cache outage costs
// availability on a path that is already degraded; allowing them costs the only
// bound on guessing.
//
// Read paths take the opposite decision. The only one today is
// Server.limitBearerAttempts in the authorization interceptor, which says so
// at its own call site; the distinction is recorded in docs2/adr/010.
//
// This is a single helper on purpose. The previous shape, `if err == nil &&
// !allowed { reject }` repeated inline at six call sites, made the fail-open
// behaviour invisible at each one and impossible to change in one place.

// enforceLimit consumes one unit from the sliding window at key and refuses the
// request when the budget is exhausted or when the limiter cannot decide.
//
// procedure and the key are logged on refusal so an operator can tell a limiter
// outage from a caller actually hitting the limit.
func (s *Server) enforceLimit(ctx context.Context, key string, limit int64, window time.Duration, procedure string) error {
	if s.cache == nil {
		// A deployment with no cache has no rate limiting at all. That is a
		// misconfiguration on an authentication endpoint, not a mode of
		// operation, so it is refused rather than silently accepted.
		s.logger.Error("rate limiter not configured; refusing authentication request",
			"procedure", procedure, "key", key)
		return connerr.Unavailable("rate limiter unavailable")
	}

	allowed, err := s.cache.Allow(ctx, key, limit, window)
	if err != nil {
		s.logger.Error("rate limiter unavailable", "error", err, "procedure", procedure, "key", key)
		return connerr.Unavailable("rate limiter unavailable")
	}
	if !allowed {
		s.logger.Warn("rate limit exceeded", "procedure", procedure, "key", key)
		return connerr.ResourceExhausted("too many requests")
	}
	return nil
}
