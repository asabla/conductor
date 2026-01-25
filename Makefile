.PHONY: all build test lint clean proto run-control-plane run-agent \
	docker-build docker-build-dashboard docker-build-all \
	docker-up docker-up-build docker-down docker-down-clean \
	docker-logs docker-logs-control-plane docker-logs-agent docker-ps \
	docker-restart docker-restart-control-plane docker-restart-agent \
	docker-test-up docker-test-down docker-test-logs \
	docker-scale-agents docker-health migrate

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Binary names
CONTROL_PLANE_BINARY=bin/control-plane
AGENT_BINARY=bin/agent
CTL_BINARY=bin/conductor-ctl

# Docker
DOCKER_COMPOSE=docker compose

# Build flags
LDFLAGS=-ldflags "-s -w"
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS_VERSION=-ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

all: lint test build

## Build targets
build: build-control-plane build-agent build-ctl

build-control-plane:
	@echo "Building control-plane..."
	@mkdir -p bin
	$(GOBUILD) $(LDFLAGS_VERSION) -o $(CONTROL_PLANE_BINARY) ./cmd/control-plane

build-agent:
	@echo "Building agent..."
	@mkdir -p bin
	$(GOBUILD) $(LDFLAGS_VERSION) -o $(AGENT_BINARY) ./cmd/agent

build-ctl:
	@echo "Building conductor-ctl..."
	@mkdir -p bin
	$(GOBUILD) $(LDFLAGS_VERSION) -o $(CTL_BINARY) ./cmd/conductor-ctl

## Test targets
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

test-short:
	@echo "Running short tests..."
	$(GOTEST) -v -short ./...

test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -race -tags=integration ./...

test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Lint targets
lint:
	@echo "Running linter..."
	$(GOLINT) run ./...

fmt:
	@echo "Formatting code..."
	$(GOFMT) -w .
	goimports -w .

## Proto targets
proto:
	@echo "Generating protobuf code..."
	buf generate

proto-lint:
	@echo "Linting protobuf files..."
	buf lint

proto-breaking:
	@echo "Checking for breaking changes..."
	buf breaking --against '.git#branch=main'

## Database targets
migrate-up:
	@echo "Running migrations up..."
	$(GOBUILD) -o bin/migrate ./cmd/control-plane && ./bin/migrate migrate up

migrate-down:
	@echo "Running migrations down..."
	$(GOBUILD) -o bin/migrate ./cmd/control-plane && ./bin/migrate migrate down

migrate-create:
	@echo "Creating new migration..."
	@read -p "Migration name: " name; \
	timestamp=$$(date +%Y%m%d%H%M%S); \
	touch migrations/$${timestamp}_$${name}.up.sql; \
	touch migrations/$${timestamp}_$${name}.down.sql; \
	echo "Created migrations/$${timestamp}_$${name}.up.sql and migrations/$${timestamp}_$${name}.down.sql"

## Docker targets
docker-build:
	@echo "Building Docker images..."
	docker build -t conductor/control-plane:latest -f Dockerfile.control-plane .
	docker build -t conductor/agent:latest -f Dockerfile.agent .

docker-build-dashboard:
	@echo "Building dashboard Docker image..."
	docker build -t conductor/dashboard:latest -f Dockerfile.dashboard .

docker-build-all: docker-build docker-build-dashboard
	@echo "All Docker images built successfully"

docker-up:
	@echo "Starting Docker Compose development environment..."
	$(DOCKER_COMPOSE) up -d

docker-up-build:
	@echo "Building and starting Docker Compose..."
	$(DOCKER_COMPOSE) up -d --build

docker-down:
	@echo "Stopping Docker Compose..."
	$(DOCKER_COMPOSE) down

docker-down-clean:
	@echo "Stopping Docker Compose and removing volumes..."
	$(DOCKER_COMPOSE) down -v --remove-orphans

docker-logs:
	$(DOCKER_COMPOSE) logs -f

docker-logs-control-plane:
	$(DOCKER_COMPOSE) logs -f control-plane

docker-logs-agent:
	$(DOCKER_COMPOSE) logs -f agent

docker-ps:
	$(DOCKER_COMPOSE) ps

docker-restart:
	@echo "Restarting services..."
	$(DOCKER_COMPOSE) restart

docker-restart-control-plane:
	$(DOCKER_COMPOSE) restart control-plane

docker-restart-agent:
	$(DOCKER_COMPOSE) restart agent

## Docker test environment
docker-test-up:
	@echo "Starting Docker Compose test environment..."
	$(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.test.yml up -d

docker-test-down:
	@echo "Stopping Docker Compose test environment..."
	$(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.test.yml down -v --remove-orphans

docker-test-logs:
	$(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.test.yml logs -f

## Docker scale agents
docker-scale-agents:
	@read -p "Number of agents: " count; \
	$(DOCKER_COMPOSE) up -d --scale agent=$$count

## Docker health check
docker-health:
	@echo "Checking service health..."
	@$(DOCKER_COMPOSE) ps --format "table {{.Name}}\t{{.Status}}\t{{.Health}}"

## Run targets (development)
run-control-plane: build-control-plane
	@echo "Running control-plane..."
	./$(CONTROL_PLANE_BINARY)

run-agent: build-agent
	@echo "Running agent..."
	./$(AGENT_BINARY)

## Clean targets
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

## Dependency targets
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

## Dashboard targets
dashboard-install:
	@echo "Installing dashboard dependencies..."
	cd web && npm install

dashboard-dev:
	@echo "Starting dashboard dev server..."
	cd web && npm run dev

dashboard-build:
	@echo "Building dashboard..."
	cd web && npm run build

dashboard-test:
	@echo "Running dashboard tests..."
	cd web && npm test

dashboard-lint:
	@echo "Linting dashboard..."
	cd web && npm run lint

## Help
help:
	@echo "Conductor Test Orchestration Platform"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Build Targets:"
	@echo "  build                    Build all binaries"
	@echo "  build-control-plane      Build control-plane binary"
	@echo "  build-agent              Build agent binary"
	@echo "  build-ctl                Build conductor-ctl binary"
	@echo ""
	@echo "Test Targets:"
	@echo "  test                     Run all tests"
	@echo "  test-short               Run short tests"
	@echo "  test-integration         Run integration tests"
	@echo "  test-coverage            Run tests with coverage report"
	@echo ""
	@echo "Lint/Format Targets:"
	@echo "  lint                     Run linter"
	@echo "  fmt                      Format code"
	@echo ""
	@echo "Proto Targets:"
	@echo "  proto                    Generate protobuf code"
	@echo "  proto-lint               Lint protobuf files"
	@echo "  proto-breaking           Check for breaking changes"
	@echo ""
	@echo "Database Targets:"
	@echo "  migrate-up               Run database migrations up"
	@echo "  migrate-down             Run database migrations down"
	@echo "  migrate-create           Create new migration"
	@echo ""
	@echo "Docker Targets:"
	@echo "  docker-build             Build control-plane and agent images"
	@echo "  docker-build-dashboard   Build dashboard image"
	@echo "  docker-build-all         Build all Docker images"
	@echo "  docker-up                Start development environment"
	@echo "  docker-up-build          Build and start development environment"
	@echo "  docker-down              Stop development environment"
	@echo "  docker-down-clean        Stop and remove volumes"
	@echo "  docker-logs              Follow all logs"
	@echo "  docker-ps                Show running containers"
	@echo "  docker-restart           Restart all services"
	@echo "  docker-health            Check service health"
	@echo "  docker-scale-agents      Scale agent instances"
	@echo ""
	@echo "Docker Test Targets:"
	@echo "  docker-test-up           Start test environment"
	@echo "  docker-test-down         Stop test environment"
	@echo "  docker-test-logs         Follow test environment logs"
	@echo ""
	@echo "Dashboard Targets:"
	@echo "  dashboard-install        Install dashboard dependencies"
	@echo "  dashboard-dev            Start dashboard dev server"
	@echo "  dashboard-build          Build dashboard"
	@echo "  dashboard-test           Run dashboard tests"
	@echo "  dashboard-lint           Lint dashboard"
	@echo ""
	@echo "Other Targets:"
	@echo "  deps                     Download dependencies"
	@echo "  tidy                     Tidy dependencies"
	@echo "  clean                    Clean build artifacts"
	@echo "  help                     Show this help"
