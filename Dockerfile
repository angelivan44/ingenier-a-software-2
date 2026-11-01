# Etapa 1: compila el servidor.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /server ./cmd/api

# Etapa 2: imagen final solo con el binario, sin herramientas de compilación.
FROM alpine:3.22
RUN adduser -D app
USER app
COPY --from=build /server /server
EXPOSE 3000
ENTRYPOINT ["/server"]
