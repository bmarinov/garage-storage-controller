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

package multicluster

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// newClusterClient returns a client for one cluster that reads through the cluster's
// cache.
// Secrets and ConfigMaps must be read directly, the controller has no cluster-wide RBAC.
func newClusterClient(cl cluster.Cluster) (client.Client, error) {
	c, err := client.New(cl.GetConfig(), client.Options{
		Scheme:     cl.GetScheme(),
		HTTPClient: cl.GetHTTPClient(),
		Mapper:     cl.GetRESTMapper(),
		Cache: &client.CacheOptions{
			Reader: cl.GetCache(),
			DisableFor: []client.Object{
				&corev1.Secret{},
				&corev1.ConfigMap{},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}
	return c, nil
}
