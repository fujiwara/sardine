GIT_VER := $(shell git describe --abbrev=0 --tags)

binary: cmd/sardine/*.go *.go go.mod go.sum
	go build -o sardine ./cmd/sardine

stringer:
	stringer -type CheckResult

test:
	go test -v ./...

dist:
	goreleaser build --snapshot --rm-dist

clean:
	rm -fr dist/* sardine

install:
	go install ./cmd/sardine

release-image: dist/
	cd dist && ln -sf sardine_linux_amd64_v1 sardine_linux_amd64 && cd -
	find dist/ -type f
	docker buildx build \
		--build-arg VERSION=${GIT_VER} \
		--platform linux/amd64,linux/arm64 \
		-f Dockerfile \
		-t ghcr.io/fujiwara/sardine:${GIT_VER} \
		--push \
		.

.PHONY: packages test lint clean setup dist install
