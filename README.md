Storage controller for S3-compatible backends.
---

## Scope

Create keys and buckets using the admin API. Bucket requests can be generated out of `Bucket` CRDs. Alternatively, create a `BucketRequest` resource directly.

The goal for this is to be as simple as it gets while covering the basic use-cases.

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
