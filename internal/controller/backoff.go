package controller

import (
	"math/bits"
	"time"
)

// ownerSecretBackoff returns the requeue interval for a missing Secret.
// Exponential backoff based on resource age up to maxInterval.
func ownerSecretBackoff(age, base, maxInterval time.Duration) time.Duration {
	if age < 0 {
		age = 0
	}
	// log2 to find bracket: floor log2( (age/base) +1 )
	n := bits.Len(uint(age/base+1)) - 1
	// shift left for 2^n
	return min(
		base<<uint(n),
		maxInterval,
	)
}
