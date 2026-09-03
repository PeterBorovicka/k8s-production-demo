.PHONY: test build lint render kind-up kind-down

test:
	cd backend && go test ./...
	python3 -m unittest scripts/test_update_chart.py

build:
	docker build -t webapp-backend:dev backend
	docker build -t webapp-frontend:dev frontend

lint:
	helm lint helm/webapp

render:
	helm template webapp helm/webapp --namespace webapp > /tmp/webapp-rendered.yaml

kind-up:
	./scripts/bootstrap-kind.sh

kind-down:
	kind delete cluster --name webapp

