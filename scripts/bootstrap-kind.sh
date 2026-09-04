#!/usr/bin/env bash
set -Eeuo pipefail

for command_name in docker kind kubectl helm; do
  command -v "${command_name}" >/dev/null || { echo "Missing required command: ${command_name}" >&2; exit 1; }
done

cluster_name="webapp"
calico_version="v3.32.2"
ingress_chart_version="4.15.1"
metrics_server_chart_version="3.14.0"
prometheus_chart_version="88.6.3"
argocd_chart_version="10.7.0"

if ! kind get clusters | grep -qx "${cluster_name}"; then
  kind create cluster --name "${cluster_name}" --config cluster/kind-config.yaml
fi

kubectl config use-context "kind-${cluster_name}"
kubectl apply -f "https://raw.githubusercontent.com/projectcalico/calico/${calico_version}/manifests/tigera-operator.yaml"
kubectl wait --for=create crd/installations.operator.tigera.io --timeout=180s
kubectl wait --for=condition=Established crd/installations.operator.tigera.io --timeout=180s
kubectl apply -f cluster/calico-custom-resources.yaml
kubectl wait --for=condition=Ready nodes --all --timeout=600s
kubectl label node "${cluster_name}-control-plane" ingress-ready=true --overwrite

helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add argo https://argoproj.github.io/argo-helm
helm repo update

helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --version "${ingress_chart_version}" \
  --namespace ingress-nginx --create-namespace \
  --set controller.kind=DaemonSet \
  --set controller.hostPort.enabled=true \
  --set controller.service.type=ClusterIP \
  --set-string controller.nodeSelector.ingress-ready=true \
  --set controller.tolerations[0].key=node-role.kubernetes.io/control-plane \
  --set controller.tolerations[0].operator=Exists \
  --set controller.tolerations[0].effect=NoSchedule \
  --wait --timeout 10m

helm upgrade --install metrics-server metrics-server/metrics-server \
  --version "${metrics_server_chart_version}" \
  --namespace kube-system \
  --set 'args[0]=--kubelet-insecure-tls' \
  --wait --timeout 5m

helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
  --version "${prometheus_chart_version}" \
  --namespace monitoring --create-namespace \
  --values monitoring/values.yaml \
  --wait --timeout 15m

helm upgrade --install argocd argo/argo-cd \
  --version "${argocd_chart_version}" \
  --namespace argocd --create-namespace \
  --wait --timeout 10m

docker build -t webapp-backend:dev backend
docker build -t webapp-frontend:dev frontend
kind load docker-image webapp-backend:dev webapp-frontend:dev --name "${cluster_name}"

kubectl create namespace webapp --dry-run=client -o yaml | kubectl apply -f -
kubectl -n webapp create secret generic webapp-secrets \
  --from-literal=api-key="${WEBAPP_API_KEY:-local-development-key}" \
  --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install webapp helm/webapp \
  --namespace webapp \
  --set images.frontend.repository=webapp-frontend \
  --set images.frontend.tag=dev \
  --set images.frontend.pullPolicy=IfNotPresent \
  --set images.backend.repository=webapp-backend \
  --set images.backend.tag=dev \
  --set images.backend.pullPolicy=IfNotPresent \
  --wait --timeout 5m

kubectl -n webapp rollout status deployment/webapp-frontend --timeout=180s
kubectl -n webapp rollout status deployment/webapp-backend --timeout=180s

echo "Ready: add '127.0.0.1 k8s-demo.local' to your hosts file, then open http://k8s-demo.local"

