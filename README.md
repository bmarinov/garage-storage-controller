Storage controller for Garage (S3-compatible) clusters.
---
[![codecov](https://codecov.io/github/bmarinov/garage-storage-controller/graph/badge.svg?token=NNYZQ863ZE)](https://codecov.io/github/bmarinov/garage-storage-controller)
# Overview

__garage-storage-controller__ handles bucket and access key management for [Garage](https://garagehq.deuxfleurs.fr/) storage clusters. 

## Project status

This project is in alpha and should be considered a technical preview. Core functionality is present. Due to the limited scope and complexity, no major changes are expected. The API will stabilize when the project reaches beta.

Important considerations:
- CRDs are still alpha and the API might need to change.
- The controller focuses on the Garage admin API for its functionality.
  - Open an issue if an important bucket config option should be exposed.
- Bucket configuration is primarily managed through the S3 API
  - Use s3 clients and client libs for e.g. policies.
  - Check [Garage: S3 compatibility status](https://garagehq.deuxfleurs.fr/documentation/reference-manual/s3-compatibility/) for details.

Limitations:
- Existing buckets currently cannot be imported.
- Accessing buckets outside the 'owning' namespace is not allowed.
- ConfigMap drift is hard to detect due to the current RBAC & security model.

## Quick start

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

## CRDs
- `Bucket`: Creates S3 buckets on the Garage cluster
- `AccessKey`: Generate credentials and store them as namespace secrets
- `AccessPolicy`: Control which keys can access a bucket

## Objectives and scope

Creating and managing storage buckets through Kubernetes API resources should be simple:
- Focus on the common use cases
- Create buckets and access keys for namespace workloads
- Keep this controller safe and easy to operate

**Note:** Deleting `Bucket` API resources will never remove the actual buckets on the storage backend. Data loss is not boring. And we aim for boring here.

## Accessing buckets in workloads

The S3 API address and the access key are available in the aforementioned ConfigMap and Secret in the namespace.

### Mounting bucket details as environment variables

The configmap as well as the secret can be consumed as environment variables:
```yaml
kind: Pod
spec:
  containers:
  - name: workload
    env:
    # s3 client config:
    - name: S3_ENDPOINT
      valueFrom:
        configMapKeyRef:
          name: bucket-sample
          key: s3-endpoint
    # bucket config values:
    - name: FOO_BUCKET_NAME
      valueFrom:
        configMapKeyRef:
          name: bucket-sample
          key: bucket-name
    - name: FOO_ACCESS_KEY
      valueFrom:
        secretKeyRef:
          name: foo-bucket-access-rw
          key: access-key-id
    - name: FOO_ACCESS_KEY_SECRET
      valueFrom:
        secretKeyRef:
          name: foo-bucket-access-rw
          key: secret-access-key
```

### Overriding the ConfigMap name

By default the controller will create a ConfigMap with the same name as the Bucket.  

If you need a different name e.g. in case of a conflict, set `configMapName`:
```yaml
apiVersion: garage.getclustered.net/v1alpha1
kind: Bucket
metadata:
  name: bucket-sample
spec:
  name: foo-global-name
  configMapName: bucket-sample-config
  maxSize: 10Gi
```

# Install

## Configuration

The controller expects the following configuration to be available as environment variables.

| Env variable            | Description                                                    |
| ------------------------| -------------------------------------------------------------- |
| GARAGE_API_ENDPOINT     | Endpoint address of the Garage admin API.                      |
| GARAGE_API_TOKEN        | Token used to authenticate requests to the Garage admin API.   |
| GARAGE_S3_API_ENDPOINT  | Endpoint address of the S3 client API provided to workloads.   |

See the [Garage Administration API](https://garagehq.deuxfleurs.fr/documentation/reference-manual/admin-api/#user-defined-api-tokens) docs on how to obtain an admin token.

## Install with Helm

### Chart values

Provide required configuration values:
```yaml
# values.yaml
config:
  garageApiEndpoint: "http://garage:3903"
  garageS3ApiEndpoint: "http://garage:3900"
```

One of `existingSecret` and `apiToken` must be set. 

**Existing secret:**

Create a secret containing the admin token:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: garage-api-token
stringData:
  api-token: "***"
```
Set the value accordingly
```yaml
existingSecret: "garage-api-token"
```

**Alternatively**, pass the API token directly as a chart value. A secret with a default name will be created:
```sh
helm install ... --set apiToken="$garage_api_token"
```

### Install
```sh
helm install garage-storage-controller \
  oci://ghcr.io/bmarinov/charts/garage-storage-controller \
  -n garage-controller-system --create-namespace \
  -f myvalues.yaml
```

### Enrolling namespaces

The controller watches custom resources cluster-wide. It requires explicit permission to read and write ConfigMaps and Secrets in tenant namespaces. Without this, reconciliation will fail:
```json
{"error": "configmaps \"...\" is forbidden: User \"system:serviceaccount:garage-controller-system:...\" cannot get resource \"configmaps\" in API group \"\" in the namespace \"default\""}
```

After installation, the ServiceAccount name is printed in the Helm notes. Use it in the RoleBinding subject below.  
The following example enrolls the `default` namespace:
```yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: garage-controller-namespace-access
  namespace: default # target namespace
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["create", "delete", "get", "list", "patch", "update"]
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["create", "delete", "get", "list", "patch", "update"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: garage-controller-namespace-access-rolebinding
  namespace: default # target namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: garage-controller-namespace-access
subjects:
- kind: ServiceAccount
  name: garage-storage-controller # controller ServiceAccount
  namespace: garage-controller-system # controller namespace 

```

## Manual installation

### Configuration

Create a configmap and a secret in the controller namespace and reference them in the deployment manifest.

Examples can be found in `config/env`:
- [kustomization.yaml](config/env/kustomization.yaml) contains env var patches for the deployment.
- [configmap.yaml](config/env/configmap.yaml) - configmap with keys for the endpoint addresses.
- [secret.yaml](config/env/api_secret.yaml) - details on creating a valid secret.

### Custom resources

Output CRD manifests to a file:
```sh
kubectl kustomize ./config/install/crds -o crds.yaml
```

Or install directly in the cluster:
```sh
kubectl apply -k ./config/install/crds
```

### RBAC

#### Controller Role and ServiceAccount

```sh
kubectl kustomize ./config/install/rbac -o rbac.yaml
```

This customization creates:
- the controller namespace and a service account
- cluster roles for CRDs
- leader election roles

#### Namespaced / tenant roles

Managing resources in a given namespace requires permissions over Secrets and ConfigMap resources.

Example with the included default namespace overlay:
```sh
kubectl kustomize ./config/rbac/namespaces/default -o default-ns-access.yaml
```

This will create a role and a role binding in one specific namespace.  
It is not recommended, but you can also provide cluster-wide access to configmaps and secrets. 

### Deployment

The controller kustomization includes an example deployment manifest:
```sh
kubectl kustomize ./config/install/controller -o deployment.yaml
```

The deployment will remain in status `CreateContainerConfigError` until the expected ConfigMap and Secret are created. See [Configuration](#configuration-1) for more details and an example.


### Full manifest

Manifests can also be rendered to a single file:
```sh
kubectl kustomize ./config/install/full > dist/install.yaml
```

# Permissions model and security

The controller needs cluster-level permissions for its CRDs. ConfigMap and Secret access can be restricted to specific namespaces.

## CRDs

The controller manages the following custom resources in the `garage.getclustered.net` group:
- `accesskeys`
- `accesspolicies`
- `buckets`

See [config/rbac/role.yaml](config/rbac/role.yaml) for the ClusterRole definition.

## ConfigMaps and Secrets

The controller stores bucket connection details in ConfigMaps and S3 credentials in Secrets. These resources are created in the same namespace as the Bucket and AccessKey resources.

Grant access and enroll a namespace (example):
```sh
kubectl apply -k "config/rbac/namespaces/${namespace}"
```

See [config/rbac/base/role_namespace_access.yaml](config/rbac/namespaces/base/role_namespace_access.yaml) for the Role definition.

**Note**: The controller does not need cluster-wide access to ConfigMaps or Secrets. Use namespace roles.

# Development

## Project setup

Scaffolding done with kubebuilder. See [docs](https://book.kubebuilder.io/reference/reference) for more info.


## Running tests

### E2E

Run suite with `make test-e2e`. Requires Kind to be installed and available.:

- Download a specific release:
```sh
version="v0.30.0"
[ "$(uname -m)" = x86_64 ] && curl -Lo ./kind https://kind.sigs.k8s.io/dl/$version/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind
```
- Or latest from Homebrew: `brew install kind`


## Debugging

### E2E Tests

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
