#!/usr/bin/env bash
# ============================================================================
# OneRole Enterprise Kubernetes (kind) Cluster Deployment Automation
# ============================================================================
set -euo pipefail

CLUSTER_NAME="onerole-cluster"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
ROOT_DIR="$(cd "${K8S_DIR}/../.." && pwd)"

echo "=== 1. Checking kind cluster: ${CLUSTER_NAME} ==="
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  echo "✓ Kind cluster '${CLUSTER_NAME}' already exists."
else
  echo "🚀 Creating kind cluster '${CLUSTER_NAME}' with ports 80/443 mapping..."
  kind create cluster --config "${K8S_DIR}/kind-cluster.yaml" --name "${CLUSTER_NAME}"
fi

echo "=== 2. Pointing kubectl context to kind-${CLUSTER_NAME} ==="
kubectl config use-context "kind-${CLUSTER_NAME}"

echo "=== 3. Loading core images into kind cluster ==="
docker pull mongo:7.0 || true
docker pull nats:2.10-alpine || true
docker pull nginx:1.27-alpine || true

kind load docker-image mongo:7.0 --name "${CLUSTER_NAME}" || true
kind load docker-image nats:2.10-alpine --name "${CLUSTER_NAME}" || true
kind load docker-image nginx:1.27-alpine --name "${CLUSTER_NAME}" || true
kind load docker-image onerole-api:latest --name "${CLUSTER_NAME}"
kind load docker-image onerole-ui:latest --name "${CLUSTER_NAME}"
kind load docker-image onerole-worker:latest --name "${CLUSTER_NAME}"

echo "=== 4. Applying Kubernetes Manifests (Namespace, ConfigMaps, Secrets, TLS) ==="
kubectl apply -f "${K8S_DIR}/00-namespace.yaml"
kubectl create secret tls onerole-tls \
  --cert="${ROOT_DIR}/deploy/certs/onerole.crt" \
  --key="${ROOT_DIR}/deploy/certs/onerole.key" \
  --namespace=onerole --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f "${K8S_DIR}/01-config-secrets.yaml"

echo "=== 5. Deploying Separated Infrastructure (MongoDB & NATS JetStream) ==="
kubectl apply -f "${K8S_DIR}/02-database-mongodb.yaml"
kubectl apply -f "${K8S_DIR}/03-messaging-nats.yaml"

echo "⏳ Waiting for MongoDB and NATS to be ready..."
kubectl rollout status statefulset/mongodb -n onerole --timeout=90s || true
kubectl rollout status statefulset/nats -n onerole --timeout=90s || true

echo "=== 6. Deploying Microservices (API, UI, Worker) ==="
kubectl apply -f "${K8S_DIR}/04-backend-api.yaml"
kubectl apply -f "${K8S_DIR}/05-frontend-ui.yaml"
kubectl apply -f "${K8S_DIR}/06-ats-worker.yaml"

echo "=== 7. Deploying NGINX Reverse Proxy for oneRole.com ==="
kubectl apply -f "${K8S_DIR}/07-nginx-reverse-proxy.yaml"
kubectl apply -f "${K8S_DIR}/08-ingress.yaml"

echo "⏳ Waiting for Microservices rollout to complete..."
kubectl rollout status deployment/onerole-api -n onerole --timeout=90s || true
kubectl rollout status deployment/onerole-ui -n onerole --timeout=90s || true
kubectl rollout status deployment/onerole-worker -n onerole --timeout=90s || true
kubectl rollout status deployment/onerole-nginx -n onerole --timeout=90s || true

echo "============================================================================"
echo "🎉 OneRole Enterprise Platform Successfully Deployed into Kubernetes!"
echo "============================================================================"
kubectl get all -n onerole
echo ""
echo "To access http://onerole.com locally, ensure your /etc/hosts has:"
echo "127.0.0.1 onerole.com www.onerole.com"
