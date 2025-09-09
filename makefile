# ==== Variables ====
DB_URL=postgres://clarity:password@localhost:5432/video_analyzer?sslmode=disable
MIGRATIONS_PATH=migrations

# ==== Go Run Commands ====
run-ai:
	go run ./cmd/ai-service

run-gateway:
	go run ./cmd/gateway

run-user:
	go run ./cmd/user-service

run-video:
	go run ./cmd/video-processor

# ==== Migrations ====
migration-create:
	@if [ -z "$(name)" ]; then \
		echo "❌ Please provide a migration name: make migration-create name=create_users_table"; \
		exit 1; \
	fi
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $(name)

migration-up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" up

migration-down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" down 1

migration-force:
	@if [ -z "$(version)" ]; then \
		echo "❌ Please provide a version number: make migration-force version=1"; \
		exit 1; \
	fi
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" force $(version)

# ==== Docker ====
docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-build:
	docker compose build

docker-logs:
	docker compose logs -f

# ==== Utilities ====
tidy:
	go mod tidy
	go fmt ./...

clean:
	docker compose down -v --remove-orphans
	go clean

# ==== Help ====
help:
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:' Makefile | cut -d: -f1
