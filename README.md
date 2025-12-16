Storage controller for S3-compatible backends.
---

## Overview

__garage-storage-controller__ handles bucket and access key management for [Garage](https://garagehq.deuxfleurs.fr/) storage clusters. 

### Quick start

```yaml
apiVersion: garage.getclustered.net/v1alpha1
kind: Bucket
metadata:
  name: bucket-sample
spec:
  name: foo-global-name
  maxSize: 9001000

---
apiVersion: garage.getclustered.net/v1alpha1
kind: AccessKey
metadata:
  name: accesskey-sample
spec:
  secretName: foo-bucket-access-rw
  neverExpires: true

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

### Manifests

```sh
kubectl apply foo/crds.yaml
kubectl apply foo/bar.yaml
```

### Helm

TBD

### Configuration

| Env variable        | Description                                                    |
| ------------------- | -------------------------------------------------------------- |
| GARAGE_API_ENDPOINT | Endpoint address of the Garage admin API.                      |
| GARAGE_API_TOKEN    | API key used to authenticate requests to the Garage admin API. |

### RBAC

## Development

### Project setup

Scaffolding done with kubebuilder. See [docs](https://book.kubebuilder.io/reference/reference) for more info.

Admin API client generated with the OpenAPI spec and oapi-codegen:

```sh
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

//go:generate go tool oapi-codegen -config cfg.yaml ../../api.yaml
```

### Running tests

#### E2E

Install Kind:

- Download a specific release:
```sh
version="v0.30.0"
[ "$(uname -m)" = x86_64 ] && curl -Lo ./kind https://kind.sigs.k8s.io/dl/$version/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind
```

- Or latest from Homebrew: `brew install kind`

