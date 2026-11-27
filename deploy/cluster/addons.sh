#!/usr/bin/env bash
# Installs the cluster add-ons (design D0.5): Traefik (ingress controller; ingress-nginx is
# retired), cert-manager with a Let's Encrypt HTTP-01 ClusterIssuer, and metrics-server. Idempotent
# (helm upgrade --install). Prints the load balancer IP and the CORE_BASE_DOMAIN it gives.
# Needs kubectl and helm with a kubeconfig for the cluster; run by .github/workflows/cluster-bootstrap.yml.
set -euo pipefail

ACME_EMAIL="${ACME_EMAIL:-}"
DOMAIN_SUFFIX="${DOMAIN_SUFFIX:-sslip.io}"   # fall back to nip.io if Let's Encrypt refuses sslip.io
LB_NAME="${LB_NAME:-eps-prototype-ams3-lb}"
TRAEFIK_VERSION="${TRAEFIK_VERSION:-43.0.0}"   # values in traefik-values.yaml follow this chart version
CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-}"
METRICS_SERVER_VERSION="${METRICS_SERVER_VERSION:-}"

ver() { [[ -n "$1" ]] && echo "--version $1" || true; }

helm repo add traefik https://traefik.github.io/charts --force-update >/dev/null
helm repo add jetstack https://charts.jetstack.io --force-update >/dev/null
helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/ --force-update >/dev/null
helm repo update >/dev/null

echo "== Traefik"
# shellcheck disable=SC2046
helm upgrade --install traefik traefik/traefik $(ver "$TRAEFIK_VERSION") \
  --namespace traefik --create-namespace --wait --timeout 10m \
  -f "$(dirname "$0")/traefik-values.yaml" \
  --set-string "service.annotations.service\.beta\.kubernetes\.io/do-loadbalancer-name=$LB_NAME"

echo "== cert-manager"
# shellcheck disable=SC2046
helm upgrade --install cert-manager jetstack/cert-manager $(ver "$CERT_MANAGER_VERSION") \
  --namespace cert-manager --create-namespace --wait --timeout 10m \
  --set crds.enabled=true

echo "== metrics-server"
# shellcheck disable=SC2046
helm upgrade --install metrics-server metrics-server/metrics-server $(ver "$METRICS_SERVER_VERSION") \
  --namespace kube-system
# Not waited on: metrics only feed autoscaling (step 12) and must not block a deploy.

echo "== ClusterIssuer letsencrypt-prod (HTTP-01 through Traefik)"
email_line=""
[[ -n "$ACME_EMAIL" ]] && email_line="    email: $ACME_EMAIL"
kubectl apply -f - <<YAML
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
$email_line
    privateKeySecretRef:
      name: letsencrypt-prod-account
    solvers:
      - http01:
          ingress:
            ingressClassName: traefik
YAML
kubectl wait --for=condition=Ready clusterissuer/letsencrypt-prod --timeout=5m
kubectl get clusterissuer letsencrypt-prod

echo "== Load balancer"
ip=""
for _ in $(seq 1 60); do
  ip=$(kubectl -n traefik get svc traefik -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
  [[ -n "$ip" ]] && break
  sleep 10
done
if [[ -z "$ip" ]]; then
  echo "The load balancer has no IP after 10 minutes." >&2
  exit 1
fi
echo "LB_IP=$ip"
echo "CORE_BASE_DOMAIN=$ip.$DOMAIN_SUFFIX"
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  echo "lb_ip=$ip" >> "$GITHUB_OUTPUT"
  echo "core_base_domain=$ip.$DOMAIN_SUFFIX" >> "$GITHUB_OUTPUT"
fi
