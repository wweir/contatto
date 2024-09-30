MAKEFLAGS += --jobs all
GO:=CGO_ENABLED=0 GODEBUG=httpmuxgo121=1 go

default: test build

test:
	${GO} vet ./...
	${GO} test ./...

build:
	${GO} build -trimpath -ldflags "\
		-X github.com/wweir/contatto/conf.Version=$(shell git describe --tags --always) \
		-X github.com/wweir/contatto/conf.Date=$(shell date +%Y-%m-%d)" \
		-o bin/contatto .
run: build
	./bin/contatto proxy --debug -c contatto.toml
install: build
	sudo install -m 0755 ./bin/contatto /bin/contatto
clean:
	rm -f ./bin/contatto
