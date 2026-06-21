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
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// HealthChecker for the Garage admin API.
type HealthChecker interface {
	Health(ctx context.Context) error
}

// UpRecorder stores the latest healthcheck result.
type UpRecorder func(isUp bool)

// defaultInterval is used when a non-positive interval is passed.
const defaultInterval = 30 * time.Second

// Run polls the Garage admin API every interval, reporting each result to record.
//
// Blocks until ctx is cancelled.
//
// When interval is <= 0, the default of 30s is used.
func Run(ctx context.Context, checker HealthChecker, record UpRecorder, interval time.Duration) {
	if interval <= 0 {
		interval = defaultInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	run(ctx, checker, record, interval, t.C)
}

// run the heartbeat loop with an injected tick source.
func run(ctx context.Context,
	checker HealthChecker,
	record UpRecorder,
	timeout time.Duration,
	ticks <-chan time.Time,
) {
	log := logf.FromContext(ctx).WithName("heartbeat")

	lastUp := true
	for {
		err := probe(ctx, checker, timeout)
		isUp := err == nil
		record(isUp)
		if isUp != lastUp {
			if isUp {
				log.Info("garage admin API reachable")
			} else {
				log.Error(err, "garage admin API unreachable")
			}
		}
		lastUp = isUp

		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

// probe limits a single healthcheck to a timeout.
// The limit shold be lt or equal to the check interval.
func probe(ctx context.Context, checker HealthChecker, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return checker.Health(ctx)
}
