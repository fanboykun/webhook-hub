package delivery

import (
	"math"
	"math/rand"
	"time"

	"github.com/fanboykun/webhook-hub/internal/config"
)

func NextAttempt(now time.Time, attempt int, retry config.RetryConfig) time.Time {
	pow := math.Pow(2, float64(max(attempt-1, 0)))
	delay := time.Duration(float64(retry.BaseDelay) * pow)
	if delay > retry.MaxDelay {
		delay = retry.MaxDelay
	}
	if retry.Jitter > 0 {
		jitter := 1 + ((rand.Float64() * 2 * retry.Jitter) - retry.Jitter)
		delay = time.Duration(float64(delay) * jitter)
	}
	return now.Add(delay)
}
