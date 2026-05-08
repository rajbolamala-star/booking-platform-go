.PHONY: help build test clean docker-up docker-down k8s-deploy k8s-delete

help:
	@echo "Booking Platform - Available targets:"
	@echo "  build         - Build all Go services"
	@echo "  test          - Run all tests"
	@echo "  docker-up     - Run all services via docker-compose"
	@echo "  docker-down   - Stop docker-compose services"
	@echo "  k8s-deploy    - Deploy to Kubernetes"
	@echo "  k8s-delete    - Delete from Kubernetes"
	@echo "  clean         - Clean build artifacts"

build:
	@for svc in services/search services/pricing services/reservation services/notification gateway; do \
		echo "Building $$svc..."; \
		(cd $$svc && go build ./...) || exit 1; \
	done

test:
	@for svc in services/search services/pricing services/reservation services/notification gateway; do \
		echo "Testing $$svc..."; \
		(cd $$svc && go test ./... -race -cover) || true; \
	done

docker-up:
	cd deploy/docker && docker-compose up --build

docker-down:
	cd deploy/docker && docker-compose down

k8s-deploy:
	kubectl apply -f deploy/k8s/booking.yaml

k8s-delete:
	kubectl delete -f deploy/k8s/booking.yaml

clean:
	@find . -name "*.exe" -delete
	@find . -name "*-service" -delete
	@echo "Cleaned"
