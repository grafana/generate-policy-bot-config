FROM --platform=${BUILDPLATFORM} golang:1.26rc2-alpine3.22@sha256:43583f46bf6dfe7c90fd1f0ac7b37ea9c2803686cc5c3a919b1a34109a3127ab AS go-builder

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
