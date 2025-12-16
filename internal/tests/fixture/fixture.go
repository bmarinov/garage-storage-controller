package fixture

import (
	"math/rand"
)

func RandAlpha(length int) string {
	b := make([]byte, length)
	for i := range b {
		base := byte('a')
		offset := rand.Intn(26) // a-z lowercase only
		b[i] = base + byte(offset)
	}
	return string(b)
}
