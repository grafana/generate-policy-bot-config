FROM --platform=${BUILDPLATFORM} golang:1.26.2-alpine3.22@sha256:6ebcc4eb3726dd548c71ad97fc8c9ab7f6d60a45fbfb7c6b734ec0489fa25c41 AS go-builder

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
