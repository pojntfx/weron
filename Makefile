# Public variables
DESTDIR ?=
PREFIX ?= /usr/local
OUTPUT_DIR ?= out
DST ?=

# Private variables
obj = webrtcfd-signaling-server webrtcfd-signaling-client
all: $(addprefix build/,$(obj))

# Build
build: $(addprefix build/,$(obj))
$(addprefix build/,$(obj)):
ifdef DST
	go build -o $(DST) ./cmd/$(subst build/,,$@)
else
	go build -o $(OUTPUT_DIR)/$(subst build/,,$@) ./cmd/$(subst build/,,$@)
endif

# Install
install: $(addprefix install/,$(obj))
$(addprefix install/,$(obj)):
	install -D -m 0755 $(OUTPUT_DIR)/$(subst install/,,$@) $(DESTDIR)$(PREFIX)/bin/$(subst install/,,$@)

# Uninstall
uninstall: $(addprefix uninstall/,$(obj))
$(addprefix uninstall/,$(obj)):
	rm $(DESTDIR)$(PREFIX)/bin/$(subst uninstall/,,$@)

# Run
$(addprefix run/,$(obj)):
	$(subst run/,,$@) $(ARGS)

# Test
test:
	go test -timeout 3600s -parallel $(shell nproc) ./...

# Benchmark
benchmark:
	go test -timeout 3600s -bench=./... ./...

# Clean
clean:
	rm -rf out internal/db

# Clean (for PostgreSQL)
clean-psql: clean
	docker rm -f webrtcfd-postgres

# Dependencies
depend:
	go install github.com/rubenv/sql-migrate/sql-migrate@latest
	go install github.com/volatiletech/sqlboiler/v4@latest
	go install github.com/volatiletech/sqlboiler-sqlite3@latest
	go install github.com/jteeuwen/go-bindata/go-bindata@latest
	sql-migrate up -env="sqlite" -config configs/sql-migrate/communities.yaml
	go generate ./internal/persisters/sqlite/...

# Dependencies (for PostgreSQL)
depend-psql: depend
	go install github.com/volatiletech/sqlboiler/v4/drivers/sqlboiler-psql@latest
	docker run -d --name webrtcfd-postgres -p 5432:5432 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=webrtcfd_communities postgres
	sleep 5
	sql-migrate up -env="psql" -config configs/sql-migrate/communities.yaml
	go generate ./internal/persisters/psql/...