package id

import (
	"crypto/rand"
	"io"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

var (
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(rand.Reader, 0)
)

func New(now time.Time) string {
	entropyMu.Lock()
	defer entropyMu.Unlock()

	return ulid.MustNew(ulid.Timestamp(now.UTC()), entropy).String()
}

func NewReader(reader io.Reader) io.Reader {
	return ulid.Monotonic(reader, 0)
}
