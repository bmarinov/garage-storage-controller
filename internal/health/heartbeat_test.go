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
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRun(t *testing.T) {
	t.Run("records every tick and logs only transitions", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		core, logs := observer.New(zap.InfoLevel)
		ctx = logf.IntoContext(ctx, zapr.NewLogger(zap.New(core)))

		checker := &fakeChecker{results: []error{nil, errors.New("boom"), nil}}
		rec := &fakeRecorder{}

		runAndWait(ctx, cancel, checker, rec.record, 2)

		if expected := []bool{true, false, true}; !slices.Equal(rec.reachability, expected) {
			t.Errorf("expected reachability %v, got %v", expected, rec.reachability)
		}
		// up -> down -> up = two transitions:
		logCount := logs.Len()
		if logCount != 2 {
			t.Errorf("expected 2 log lines got %d", logCount)
		}
		errLogCount := logs.FilterLevelExact(zapcore.ErrorLevel).Len()
		if errLogCount != 1 {
			t.Errorf("expected 1 error-level log on conn loss got %d", errLogCount)
		}
		infoCount := logs.FilterLevelExact(zapcore.InfoLevel).Len()
		if infoCount != 1 {
			t.Errorf("expected 1 info log for the recovery, got %d", infoCount)
		}
	})

	t.Run("passing zero interval", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		checker := &fakeChecker{results: []error{nil}, onLast: cancel}
		rec := &fakeRecorder{}

		Run(ctx, checker, rec.record, 0)

		if len(rec.reachability) != 1 || !rec.reachability[0] {
			t.Errorf("expected single true value got %v", rec.reachability)
		}
	})

	t.Run("healthy steady state", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		core, logs := observer.New(zap.InfoLevel)
		ctx = logf.IntoContext(ctx, zapr.NewLogger(zap.New(core)))

		checker := &fakeChecker{results: []error{nil, nil, nil}}

		runAndWait(ctx, cancel, checker, (&fakeRecorder{}).record, 2)

		if logs.Len() != 0 {
			t.Errorf("expected 0 log lines in steady state, got %d", logs.Len())
		}
	})
}

// runAndWait deterministically runs the heartbeat for 1 + tickCount operations.
// Cancels and waits for run to return.
func runAndWait(
	ctx context.Context,
	cancel context.CancelFunc,
	checker HealthChecker,
	record UpRecorder,
	tickCount int,
) {
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		run(ctx, checker, record, time.Minute, ticks)
		close(done)
	}()
	for range tickCount {
		ticks <- time.Now()
	}
	cancel()
	<-done
}

type fakeChecker struct {
	results []error
	i       int
	onLast  func()
}

func (f *fakeChecker) Health(context.Context) error {
	err := f.results[f.i]
	f.i++
	if f.i == len(f.results) && f.onLast != nil {
		f.onLast()
	}
	return err
}

type fakeRecorder struct {
	reachability []bool
}

func (f *fakeRecorder) record(isUp bool) {
	f.reachability = append(f.reachability, isUp)
}
