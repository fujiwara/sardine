LATEST_TAG := $(shell git describe --abbrev=0 --tags)

setup:
	go get \
		github.com/laher/goxc \
		github.com/tcnksm/ghr \
		github.com/golang/lint/golint \
		github.com/golang/dep
	go get -d -t ./...
	dep ensure

test: setup
	go test -v ./...

lint: setup
	go vet ./...
	golint -set_exit_status ./...

dist: setup
	goxc

clean:
	rm -fr dist/*

release: dist
	ghr -u fujiwara -r sardine $(LATEST_TAG) dist/snapshot/

.PHONY: packages test lint clean setup dist
