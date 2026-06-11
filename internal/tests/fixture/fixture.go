// Copyright 2025.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
