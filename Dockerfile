FROM --platform=${BUILDPLATFORM} golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS go-builder

RUN apk add --no-cache 'ca-certificates>=20261020-r0' \
    && update-ca-certificates

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
