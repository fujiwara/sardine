LATEST_TAG := $(shell git describe --abbrev=0 --tags)

binary: cmd/sardine/sardine

stringer:
	stringer -type CheckResult

test:
	go test -v ./...

lint:
	go vet ./...
	golint -set_exit_status ./...

dist:
	CGO_ENABLED=0 goxz -pv=$(LATEST_TAG) -os=darwin,linux,windows -build-ldflags="-w -s" -arch=amd64 -d=dist -z ./cmd/sardine

clean:
	rm -fr dist/* cmd/sardine/sardine

release: dist
	ghr -u fujiwara -r sardine $(LATEST_TAG) dist/snapshot/

cmd/sardine/sardine: *.go go.* cmd/sardine/*.go
	cd cmd/sardine && go build .

install: cmd/sardine/sardine
	install cmd/sardine/sardine $(GOPATH)/bin

.PHONY: packages test lint clean setup dist install
