# Public variables
DESTDIR ?=
PREFIX ?= /usr/local
OUTPUT_DIR ?= out
DST ?=

# Private variables
obj = wrtcsgl wrtcchat wrtcmgr wrtctkn wrtceth
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
	docker rm -f webrtcfd-postgres webrtcfd-redis

# Dependencies
depend:
	docker run -d --name webrtcfd-postgres -p 5432:5432 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=webrtcfd_communities postgres
	docker run -d --name webrtcfd-redis -p 6379:6379 redis
	docker exec webrtcfd-postgres bash -c 'until pg_isready; do sleep 1; done'
	go install github.com/rubenv/sql-migrate/sql-migrate@latest
	go install github.com/volatiletech/sqlboiler/v4@latest
	go install github.com/jteeuwen/go-bindata/go-bindata@latest
	go install github.com/volatiletech/sqlboiler/v4/drivers/sqlboiler-psql@latest
	sql-migrate up -env="psql" -config configs/sql-migrate/communities.yaml
	go generate ./internal/persisters/psql/...
