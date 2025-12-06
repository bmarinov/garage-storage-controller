Storage controller for S3-compatible backends.
---

## Overview

Creating and managing storage buckets through Kubernetes API resources should be simple and boring.

## Scope
- Create buckets and access keys for namespace workloads with a few CRDs.
- Expose keys through Kubernetes secrets.
- Focus on the common use cases while keeping this controller easy to operate.

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

(API key)


## Development

### Project setup

Scaffolding done with kubebuilder. See [docs](https://book.kubebuilder.io/reference/reference) for more info.

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

