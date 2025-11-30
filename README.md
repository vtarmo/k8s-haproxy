# HAProxy K8s Sync Controller

A lightweight Go controller that watches a single Kubernetes ingress Service (`Endpoints`/`EndpointSlice`) and keeps an HAProxy backend server list in sync using the HAProxy Data Plane API transactions.

- Supports HAProxy Data Plane API **v3.0+** (HAProxy 2.6+ with s6 packaging).
- Deployable in-cluster as a simple Deployment (manifests in `deploy/`) or via Helm chart (`charts/haproxy-k8s-sync/`).

## How It Works

1. Watches `Endpoints` and `EndpointSlices` for the configured ingress Service.
2. Resolves server addresses to Node InternalIPs and optional fixed backend port (for NodePort setups).
3. Reconciles HAProxy backend servers inside a transaction: begin → upsert servers → update backend settings (balance, tcp-check, default-server PROXY v2 if enabled) → commit.

## Configuration

Environment variables (set via ConfigMap/Secret in manifests/Helm):

| Variable | Description |
| --- | --- |
| `INGRESS_NAMESPACE` | Namespace of ingress Service to watch (default `ingress-nginx`). |
| `INGRESS_SERVICE_NAME` | Ingress Service name (default `ingress-nginx`). |
| `HAPROXY_DATAPLANE_URL` | HAProxy Data Plane API base URL (v3.0+). |
| `HAPROXY_DATAPLANE_USERNAME` / `HAPROXY_DATAPLANE_PASSWORD` | Basic auth credentials (optional). |
| `HAPROXY_DATAPLANE_TOKEN` | Bearer token (optional alternative to basic auth). |
| `HAPROXY_BACKEND_NAME` | Target HAProxy backend name (defaults to ingress service name). |
| `HAPROXY_BACKEND_PORT` | Override backend port (useful for NodePort). |
| `HAPROXY_SEND_PROXY_V2` | `true` to enable `default-server send-proxy-v2` with tcp-check. |
| `RESYNC_PERIOD` | Informer resync (default `30s`). |

## Deployment

### Manifests
Use the provided manifests in `deploy/`:
```bash
kubectl apply -f deploy/configmap.yaml
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/deployment.yaml
```

### Helm
Package or use the published Helm chart in `charts/haproxy-k8s-sync/`:
```bash
helm install haproxy-sync charts/haproxy-k8s-sync \
  --namespace ingress-nginx --create-namespace \
  --set env.haproxy.dataplaneURL=http://haproxy:5555 \
  --set env.haproxy.backendName=be_ingress_https
```
See `charts/haproxy-k8s-sync/values.yaml` for all tunables.

## Requirements

- Kubernetes cluster with `Endpoints`/`EndpointSlice` APIs available.
- HAProxy Data Plane API v3.0+ (HAProxy 2.6+ s6 builds) reachable from the controller.
- RBAC rights: `get/list/watch` on Endpoints, EndpointSlices, and Nodes in the target cluster.

## HAProxy / Data Plane API notes

- Run HAProxy in **master-worker** mode (s6 images do this by default).
- Create a Data Plane API user in `haproxy.cfg`:
  ```cfg
  userlist dataplaneapi
    user admin insecure-password <replace-with-password>
  ```
- Example server template in the backend (pre-created backend name must match `HAPROXY_BACKEND_NAME`):
  ```cfg
  backend be_ingress_https
    mode tcp
    balance roundrobin
    option tcp-check
    default-server send-proxy-v2 check inter 5s rise 2 fall 2
    server-template ingress 5 127.0.0.1:65535 check disabled
  ```
- Minimal `dataplaneapi.yml` example:
  ```yaml
  config_version: 2
  dataplaneapi:
    host: 0.0.0.0
    port: 5555
    advertised:
      api_address: ""
      api_port: 0
    scheme:
    - http
    userlist:
      userlist: dataplaneapi
      userlist_file: ""
    transaction:
      transaction_dir: /tmp/haproxy
  haproxy:
    config_file: /usr/local/etc/haproxy/haproxy.cfg
    haproxy_bin: /usr/local/sbin/haproxy
    master_runtime: /var/run/haproxy-master.sock
    master_worker_mode: true
    reload:
      reload_delay: 5
      service_name: haproxy
      reload_strategy: s6
  log_targets:
  - log_to: stdout
    log_level: info
  ```

## Development

Prerequisites: Go 1.24+.

```bash
go test ./...
```

Build locally:
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/haproxy-k8s-sync-controller ./cmd/haproxy-k8s-sync-controller
```

## Notes

- Server names default to Kubernetes Node names (fallback to IP) and use the configured backend port.
- Health checks: `adv_check` set to `tcp-check`, `balance` set to `roundrobin`, and default-server can enable `send-proxy-v2` when configured.
