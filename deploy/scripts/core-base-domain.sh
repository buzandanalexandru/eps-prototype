#!/usr/bin/env bash
# Prints CORE_BASE_DOMAIN (<traefik load balancer ip>.sslip.io) from the cluster.
set -euo pipefail
ip=$(kubectl -n traefik get svc traefik -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
[[ -n "$ip" ]] || { echo "Traefik has no load balancer IP: run the cluster-bootstrap workflow." >&2; exit 1; }
echo "$ip.${DOMAIN_SUFFIX:-sslip.io}"
