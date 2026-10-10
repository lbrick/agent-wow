# Builds one classic module image from the repository root:
#   podman build -f modules/classic/Dockerfile --build-arg MODULE=world -t localhost/agent-wow-classic-world:dev .
FROM docker.io/library/golang:1.27.1 AS build
ARG MODULE
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /module ./modules/classic/${MODULE}/cmd

FROM scratch
COPY --from=build /module /module
ENTRYPOINT ["/module"]
