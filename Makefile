.PHONY: help fetch-h2 bridge frontend test integration vet build dev clean

help:
	@echo "fetch-h2    Download the pinned H2 engine into assets/"
	@echo "bridge      Compile the embedded JDBC bridge (requires a JDK)"
	@echo "frontend    Install deps and build the React bundle"
	@echo "test        Run Go unit tests"
	@echo "integration Run real-H2 integration tests (TAILSQL_INTEGRATION=1)"
	@echo "vet         Run go vet"
	@echo "build       Wails production build (requires Wails CLI)"
	@echo "dev         Wails development mode"

fetch-h2:
	bash ./scripts/fetch-h2.sh

bridge:
	bash ./scripts/build-bridge.sh

frontend:
	cd frontend && npm install --include=dev && npm run build

test:
	go test ./...

integration:
	TAILSQL_INTEGRATION=1 go test ./internal/dbmanager -run Integration -v

vet:
	go vet ./...

build:
	bash ./scripts/build.sh

dev:
	wails dev

clean:
	rm -rf build/bin frontend/dist
