FROM --platform=${BUILDPLATFORM} golang:1.25.6-alpine3.22@sha256:ab0033b336123effda5e57bda16c24334cf91f30695fa646ac67f1b2093877df AS go-builder

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
