FROM --platform=${BUILDPLATFORM} golang:1.26rc3-alpine3.22@sha256:b6ea8b9c20d995bb6a920497d5cdd8b020c6d3cc8ace5fb99a8bfbb51dc787b5 AS go-builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /usr/src/generate-policy-bot-config

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -v -o generate-policy-bot-config cmd/generate-policy-bot-config/main.go

FROM scratch

COPY --from=go-builder /usr/src/generate-policy-bot-config /usr/bin

ENTRYPOINT [ "/usr/bin/generate-policy-bot-config" ]
