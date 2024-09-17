MAKEFLAGS += --jobs all
GO:=CGO_ENABLED=0 GODEBUG=httpmuxgo121=1 go

default: test build

test:
	${GO} vet ./...
	${GO} test ./...

build: test
	${GO} build -ldflags "\
		-X main.version=$(shell git describe --tags --always) \
		-X main.date=$(shell date +%Y-%m-%d)" \
		-o bin/contatto .
run: build
	./bin/contatto proxy -c config.toml

clean:
	rm -f ./bin/contatto
