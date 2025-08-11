.PHONY: build run test clean docker-build docker-run

SERVICE_NAME=api-gateway
PORT=8080

build:
	go build -o $(SERVICE_NAME) ./cmd/$(SERVICE_NAME)

run: build
	./$(SERVICE_NAME)

test:
	go test -v ./...

clean:
	rm -f $(SERVICE_NAME)

docker-build:
	docker build -t $(SERVICE_NAME):latest .

docker-run: docker-build
	docker run -p $(PORT):$(PORT) $(SERVICE_NAME):latest

tidy:
	go mod tidy

dev:
	air -c .air.toml
