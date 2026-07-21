BINARY_NAME=regiusApp

build:
	@go mod vendor
	@echo "Building Regius..."
	@go build -o tmp/${BINARY_NAME} .
	@echo "Regius built!"

run: build
	@echo "Starting Regius..."
	@./tmp/${BINARY_NAME} &
	@echo "Regius started!"

clean:
	@echo "Cleaning..."
	@go clean
	@rm tmp/${BINARY_NAME}
	@echo "Cleaned!"

test:
	@echo "Testing..."
	@go test ./...
	@echo "Done!"

start: stop run

stop:
	@echo "Stopping Regius..."
	@-pkill -SIGTERM -f "./tmp/${BINARY_NAME}"
	@echo "Stopped Regius"

restart: stop start

start_compose:
	docker compose up -d

stop_compose:
	docker compose down

coverprofile:
	go test -coverprofile=coverage.out
