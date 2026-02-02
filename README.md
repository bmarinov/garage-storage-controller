Storage controller for Garage (S3-compatible) clusters.
---
[![codecov](https://codecov.io/github/bmarinov/garage-storage-controller/graph/badge.svg?token=NNYZQ863ZE)](https://codecov.io/github/bmarinov/garage-storage-controller)
## Overview

__garage-storage-controller__ handles bucket and access key management for [Garage](https://garagehq.deuxfleurs.fr/) storage clusters. 

### Project status

This project is in alpha and should be considered a technical preview. Core functionality is present, but some edge cases and common errors are not yet handled.

CRDs are still alpha, following is subject to change:
- Naming conventions for created (owned) resources.
- Importing existing resources (e.g. buckets).
- Differentiate between exact and derived (pre-/suffixed) resource names.

API will stabilize as soon as the project hits beta state.

### Quick start

```yaml
apiVersion: garage.getclustered.net/v1alpha1
kind: Bucket
metadata:
  name: bucket-sample
spec:
  name: foo-global-name
  maxSize: 10Gi

---
apiVersion: garage.getclustered.net/v1alpha1
kind: AccessKey
metadata:
  name: accesskey-sample
spec:
  secretName: foo-bucket-access-rw

---
apiVersion: garage.getclustered.net/v1alpha1
kind: AccessPolicy
metadata:
  name: accesspolicy-sample
spec:
  accessKey: accesskey-sample
  bucket: bucket-sample
  permissions:
    read: true
    write: true
    owner: false

```

This will create a bucket and an access key on Garage. Permissions will be set via the admin API.

Bucket configuration and access key will be stored in Kubernetes objects:
```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: bucket-sample
data:
  bucket-name: "foo-global-name-abcd1234"
  s3-endpoint: "https://s3.garage.foo.net"

---
apiVersion: v1
kind: Secret
metadata:
  name: foo-bucket-access-rw
data:
  access-key-id: "ABC..."
  secret-access-key: "..."
```

### CRDs
- `Bucket`: Creates S3 buckets on the Garage cluster
- `AccessKey`: Generate credentials and store them as namespace secrets
- `AccessPolicy`: Control which keys can access a bucket

### Objectives and scope

Creating and managing storage buckets through Kubernetes API resources should be simple:
- Focus on the common use cases
- Create buckets and access keys for namespace workloads
- Keep this controller safe and easy to operate

**Note:** Deleting `Bucket` API resources will never remove the actual buckets on the storage backend. Data loss is not boring. And we aim for boring here.


## Install

### Configuration

| Env variable            | Description                                                    |
| ------------------------| -------------------------------------------------------------- |
| GARAGE_API_ENDPOINT     | Endpoint address of the Garage admin API.                      |
| GARAGE_API_TOKEN        | API key used to authenticate requests to the Garage admin API. |
| GARAGE_S3_API_ENDPOINT  | Endpoint address of the S3 client API provided to workloads.   |

### Manifests

#### Custom resources

Output CRD manifests to a file:
```sh
kubectl kustomize ./config/crd -o crds.yaml
```

Or install directly in the cluster:
```sh
kubectl apply -k ./config/crd
```

### Helm

TBD

## Permissions model and security

The controller needs cluster-level permissions for its CRDs. ConfigMap and Secret access can be restricted to specific namespaces.

### CRDs

The controller manages the following custom resources in the `garage.getclustered.net` group:
- `accesskeys`
- `accesspolicies`
- `buckets`

See [config/rbac/role.yaml](config/rbac/role.yaml) for the ClusterRole definition.

### ConfigMaps and Secrets

The controller stores bucket connection details in ConfigMaps and S3 credentials in Secrets. These resources are created in the same namespace as the Bucket and AccessKey resources.

Grant access and enroll a namespace (example):
```sh
kubectl apply -k "config/rbac/namespaces/${namespace}"
```

See [config/rbac/base/role_namespace_access.yaml](config/rbac/base/role_namespace_access.yaml) for the Role definition.

**Note**: The controller does not need cluster-wide access to ConfigMaps or Secrets. Use namespace roles.

## Development

### Project setup

Scaffolding done with kubebuilder. See [docs](https://book.kubebuilder.io/reference/reference) for more info.


### Running tests

#### E2E

Run suite with `make test-e2e`. Requires Kind to be installed and available.:

- Download a specific release:
```sh
version="v0.30.0"
[ "$(uname -m)" = x86_64 ] && curl -Lo ./kind https://kind.sigs.k8s.io/dl/$version/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind
```
- Or latest from Homebrew: `brew install kind`



### Debugging

#### E2E Tests

Setup environment with `make setup-test-e2e` and launch tests in e2e packge.

VS Code launch profile:
```json
{
  "configurations": [
    {
        "name": "Debug E2E Tests",
        "type": "go",
        "request": "launch",
        "mode": "test",
        "program": "${workspaceFolder}/test/e2e",
        "env": {
            "KIND_CLUSTER": "garage-storage-controller-test-e2e",
            "RUN_E2E": "true"
        }
    }
  ]
}
```
