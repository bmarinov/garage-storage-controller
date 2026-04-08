garage-storage-controller
---

Helm chart for the deployment of the Garage storage controller.

## Versioning

The chart and app are versioned independently. 

| Chart Version | App Version  |
|---------------|--------------|
| 0.2.1         | v0.2.2       |
| 0.3.0         | v0.2.2       |
| 0.3.1         | v0.2.2       |
| 0.3.2         | v0.3.0       |

## Values

See [values.yaml](./values.yaml) for a full list of supported values and their defaults.

### Controller Config

Both values are **required**:
| Key | Default | Description |
|-----|---------|-------------|
| `config.garageApiEndpoint` | `""` | Garage admin API endpoint |
| `config.garageS3ApiEndpoint` | `""` | Garage S3 API endpoint |


### Garage Authentication

The API token should be supplied in **exactly** one way:

| Key | Default | Description |
|-----|---------|-------------|
| `existingSecret` | `""` | Name of a pre-existing Secret. Must contain an `api-token` key |
| `apiToken` | `""` | Plain-text token; the chart will create the Secret |

### Service Account
The name of the service account, relevant for RBAC and tenant namespace access.

| Key | Default | Description |
|-----|---------|-------------|
| `serviceAccount.name` | `""` | Service account name; generated from the fullname template if left empty |
| `serviceAccount.annotations` | `{}` | Annotations to add to the service account |

### Deployment

| Key | Default | Description |
|-----|---------|-------------|
| `replicas` | `1` | Number of replicas |
| `podAnnotations` | `{}` | Extra annotations for the pod |
| `podLabels` | `{}` | Extra labels for the pod |

### Resources

| Key | Default | Description |
|-----|---------|-------------|
| `resources.requests.cpu` | `50m` | CPU request |
| `resources.requests.memory` | `64Mi` | Memory request |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `128Mi` | Memory limit |
 
### Security Context

| Key | Default | Description |
|-----|---------|-------------|
| `podSecurityContext` | see [values.yaml](./values.yaml) | Pod-level security context |

The container security context cannot be configured. It is currently set as follows:
```yaml
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop: [ALL]
  readOnlyRootFilesystem: true
```
