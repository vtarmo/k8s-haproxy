# Helm chart – haproxy-k8s-sync

Helm-repo lisamiseks (pärast GitHub Actions publitseerimist gh-pages harule):

```bash
helm repo add vtarmo-haproxy https://vtarmo.github.io/k8s-haproxy
helm repo update
helm install haproxy-sync vtarmo-haproxy/haproxy-k8s-sync \
  --namespace ingress-nginx --create-namespace
```

Vaikimisi väärtused on failis `charts/haproxy-k8s-sync/values.yaml`. Override’i jaoks: `-f myvalues.yaml` või `--set image.tag=v1.0.0`.
