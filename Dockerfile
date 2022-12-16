FROM alpine:3.16.2
RUN apk add --no-cache ca-certificates
ARG VERSION
ARG TARGETARCH
ADD dist/sardine_linux_${TARGETARCH}/sardine /usr/local/bin/sardine
ENV SARDINE_CONFIG=/etc/sardine/config.yaml
ENV SARDINE_DEBUG=false
ENTRYPOINT ["/usr/local/bin/sardine"]
