.PHONY: build
build:
	@go build -o bin/mrs ./cmd/api

.PHONY: run
run: build
	@./bin/mrs

.PHONY: test
test:
	@go test -race -v ./...

.PHONY: migrate_create
migrate_create:
	@migrate -database=${DB_DSN} create -seq -ext=sql -dir=./migrations $(name)

.PHONY: migrate_up
migrate_up:
	@migrate -database=${DB_DSN} -path=./migrations up 1


.PHONY: migrate_down
migrate_down:
	@migrate -database=${DB_DSN} -path=./migrations down 1


.PHONY: migrate_force
migrate_force:
	@migrate -database=${DB_DSN} -path=./migrations force $(version)

.PHONY: migrate_version
migrate_version:
	@migrate -database=${DB_DSN} -path=./migrations version