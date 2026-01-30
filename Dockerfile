FROM --platform=${BUILDPLATFORM} golang:1.25.6-alpine3.22@sha256:fa3380ab0d73b706e6b07d2a306a4dc68f20bfc1437a6a6c47c8f88fe4af6f75 AS go-builder

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
