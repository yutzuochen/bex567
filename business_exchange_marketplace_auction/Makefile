.PHONY: run build clean tidy migrate migrate-down migrate-status finalize-job docker-up docker-down

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server
	go build -o bin/migrate ./cmd/migrate  
	go build -o bin/finalize-job ./cmd/finalize-job

clean:
	rm -rf bin

tidy:
	go mod tidy

migrate:
	go run ./cmd/migrate -action=up

migrate-down:
	go run ./cmd/migrate -action=down

migrate-status:
	go run ./cmd/migrate -action=status

finalize-job:
	go run ./cmd/finalize-job

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down -v

test:
	go test ./...

test-coverage:
	go test -cover ./...