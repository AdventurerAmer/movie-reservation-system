.PHONY: build
build:
	@go build -o bin/mrs ./cmd/api

.PHONY: run
run: build
	@./bin/mrs

.PHONY: test
test:
	@go test -race -v ./...

.PHONY: create_db
create_db:
	@psql ${DB_DSN} -tc "SELECT 1 FROM pg_database WHERE datname = 'mrs'" | grep -q 1 || psql -U postgres -c "CREATE DATABASE mrs"

.PHONY: migrate_create
migrate_create:
	@migrate -database=${DB_DSN} create -seq -ext=sql -dir=./migrations $(name)

.PHONY: migrate_up
migrate_up:
	@migrate -database=${DB_DSN} -path=./migrations up 1

.PHONY: migrate_to_latest
migrate_to_latest:
	@migrate -database=${DB_DSN} -path=./migrations up

.PHONY: migrate_down
migrate_down:
	@migrate -database=${DB_DSN} -path=./migrations down 1

.PHONY: migrate_force
migrate_force:
	@migrate -database=${DB_DSN} -path=./migrations force $(version)

.PHONY: migrate_version
migrate_version:
	@migrate -database=${DB_DSN} -path=./migrations version

.PHONY: generate_tls_cert
generate_tls_cert:
	@openssl genrsa -out tls/key.pem 2048
	@openssl req -new -key tls/key.pem -out tls/cert.pem
	@openssl x509 -req -days 365 -in tls/cert.pem -signkey tls/key.pem -out tls/cert.pem
	@openssl x509 -in tls/cert.pem -text -noout

.PHONY: stripe_listen
stripe_listen:
	@stripe listen --forward-to https://localhost:${PORT}

.PHONY: generate_docs
generate_docs:
	@swag fmt -d ./cmd/api
	@swag init -d ./cmd/api --parseDependency