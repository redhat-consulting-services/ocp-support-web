# Build stage
ARG BUILDER_IMAGE=registry.redhat.io/ubi9/go-toolset:latest
ARG RUNTIME_IMAGE=registry.redhat.io/openshift4/ose-cli-rhel9:latest
ARG APP_VERSION=dev

FROM ${BUILDER_IMAGE} AS builder

ARG APP_VERSION
COPY . .
RUN go build -ldflags="-X main.version=${APP_VERSION}" -o ocp-support-web .

# Runtime stage - ose-cli provides oc binary for must-gather / etcd commands
FROM ${RUNTIME_IMAGE}

COPY --from=builder /opt/app-root/src/ocp-support-web /usr/local/bin/ocp-support-web

RUN mkdir -p /tmp/ocp-support-web/gather && \
    chown -R 1001:0 /tmp/ocp-support-web && \
    chmod -R g+rwX /tmp/ocp-support-web

USER 1001

EXPOSE 8080

ENTRYPOINT ["ocp-support-web"]
