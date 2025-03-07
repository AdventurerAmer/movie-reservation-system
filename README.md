# Movie Reservation System Restful API

## Description

	MRS An educational movie reservation system restful API.

## Goal

	The project's goal was purely educational, and I had fun learning about RESTful APIs,
    system design, complex queries, transactions, and payment gateway integration.

## Features

    - Configuration with environment variables.
    - CORS
    - Rate limiting
    - TLS
    - Payment gateway integration (Stripe)
    - Docs generation with swagger

## Usage

- install [go](https://go.dev/)
- install [PostgreSQL](https://www.postgresql.org/)   

- install [migrate](https://github.com/golang-migrate/migrate)
```bash
go install -tags "postgres" github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

- install [openssl](https://github.com/openssl/openssl)
- install [stripe-cli](https://docs.stripe.com/stripe-cli)

- install [swag](https://github.com/swaggo/swag)
```bash
go install github.com/swaggo/swag/cmd/swag@latest
```

- The ===Makefile=== relies on environment variables so make sure it's loaded and check out the ===.exampleenv=== file for all the variables you need to fill.
```bash
source .env
```

- create the database
```bash
make create_db
```

- run database migrations
```bash
make migrate_to_latest
```

- generate TLS certificate
```bash
make generate_tls_cert
```

- generate docs
```bash
make generate_docs
```

- stripe web hook
```bash
make stripe_listen
```

- get dependencies
```bash
go mod tidy
```

- run server
```bash
make run
```

- Docs endpoint
https://localhost:{port}/v1/docs/