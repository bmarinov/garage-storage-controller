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

package health

import (
	"context"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/bmarinov/garage-storage-controller/internal/garage"
)

type GarageAdminAuth interface {
	CurrentTokenInfo(ctx context.Context) (garage.AdminTokenInfo, error)
}

// PreflightCheck logs the result of a one-time admin token check at startup.
// It never returns an error or cause the controller to exit.
func PreflightCheck(ctx context.Context, garageAdm GarageAdminAuth) {
	log := logf.FromContext(ctx).WithName("preflight")
	info, err := garageAdm.CurrentTokenInfo(ctx)
	switch {
	case err != nil:
		log.Info("garage admin API preflight failed, starting anyway; "+
			"check GARAGE_API_ENDPOINT / GARAGE_API_TOKEN", "error", err)
	case info.Expired:
		log.Info("garage admin API reachable but token EXPIRED",
			"token", info.Name, "expiration", info.Expiration)
	default:
		kv := []any{"token", info.Name}
		if info.Expiration != nil {
			kv = append(kv, "expiration", *info.Expiration)
		}
		kv = append(kv, "scopes", info.Scope)
		log.Info("garage admin API reachable, token accepted", kv...)
	}
}
