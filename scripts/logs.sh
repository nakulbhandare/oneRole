#!/usr/bin/env bash
set -e

SERVICE="${1:-api}"

case "$SERVICE" in
  api|backend)
    echo "=== Streaming OneRole Backend API Logs (Docker) ==="
    docker logs -f onerole_api
    ;;
  ui|frontend)
    echo "=== Streaming OneRole UI Logs (Docker) ==="
    docker logs -f onerole_ui
    ;;
  worker|workers|chromium)
    echo "=== Streaming Chromium ATS Submitter Worker Logs (Docker) ==="
    docker logs -f onerole-local-chromium-worker-1
    ;;
  es|elastic|elasticsearch)
    echo "=== Streaming Elasticsearch Logs (Docker) ==="
    docker logs -f onerole_elasticsearch
    ;;
  k8s-api)
    echo "=== Streaming Kubernetes API Pod Logs ==="
    kubectl logs -n onerole -l app.kubernetes.io/name=onerole-api -f
    ;;
  k8s-ui)
    echo "=== Streaming Kubernetes UI Pod Logs ==="
    kubectl logs -n onerole -l app.kubernetes.io/name=onerole-ui -f
    ;;
  *)
    echo "Usage: ./scripts/logs.sh [api|ui|worker|es|k8s-api|k8s-ui]"
    exit 1
    ;;
esac
