#!/usr/bin/env bash
set -Eeuo pipefail

helm lint helm/webapp
helm template webapp helm/webapp --namespace webapp >/tmp/webapp-rendered.yaml
python3 -m unittest scripts/test_update_chart.py
(cd backend && go test ./...)
docker build -t webapp-backend:verify backend
docker build -t webapp-frontend:verify frontend

echo "All available project checks passed."

