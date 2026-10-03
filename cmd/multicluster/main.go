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

// Command multicluster runs the Garage controllers through multicluster-runtime.
// It uses the single provider: one cluster, the one from the kubeconfig.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins:
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/providers/single"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/config"
	"github.com/bmarinov/garage-storage-controller/internal/controller/multicluster"
	"github.com/bmarinov/garage-storage-controller/internal/garage"
	"github.com/bmarinov/garage-storage-controller/internal/health"
)

var setupLog = ctrl.Log.WithName("setup")

func main() {
	var metricsAddr string
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0",
		"The address the metrics endpoint binds to, over plain HTTP. 0 disables the metrics endpoint.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	cfg, err := config.Load()
	if err != nil {
		setupLog.Error(err, "loading controller config")
		os.Exit(1)
	}

	scheme, err := newScheme()
	if err != nil {
		setupLog.Error(err, "building scheme")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.Secret{},
					&corev1.ConfigMap{},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	mcMgr, err := mcmanager.WithMultiCluster(mgr, single.New("local", mgr))
	if err != nil {
		setupLog.Error(err, "unable to create multicluster manager")
		os.Exit(1)
	}

	garageMetrics := garage.NewMetrics()
	crmetrics.Registry.MustRegister(garageMetrics.Collectors()...)

	garageClient := garage.NewClient(
		cfg.GarageAPIEndpoint,
		cfg.GarageAPIToken,
		garage.WithMetrics(garageMetrics),
	)

	signalCtx := ctrl.SetupSignalHandler()

	preflightCtx, cancel := context.WithTimeout(signalCtx, 10*time.Second)
	defer cancel()
	health.PreflightCheck(preflightCtx, garageClient)

	if err := multicluster.Setup(mcMgr, multicluster.Garage{
		Buckets:    garageClient.BucketClient,
		Ownership:  garageClient.PermissionClient,
		AccessKeys: garageClient.AccessKeyClient,
		S3Endpoint: cfg.GarageS3Endpoint,
	}); err != nil {
		setupLog.Error(err, "unable to set up controllers")
		os.Exit(1)
	}

	if err := mcMgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mcMgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	go health.Run(signalCtx, garageClient, garageMetrics.SetAPIUp, 30*time.Second)

	setupLog.Info("starting manager")
	if err := mcMgr.Start(signalCtx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func newScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding client-go types: %w", err)
	}
	if err := garagev1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding garage types: %w", err)
	}
	return scheme, nil
}
