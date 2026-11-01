# Acortador de enlaces

Servicio web en Go para crear enlaces cortos, redirigir a la URL original y consultar cuántos clics recibió cada enlace.

Proyecto del curso Ingeniería de Software 2, Universidad Continental.

## Requisitos

- Go 1.24 o superior

## Instalación

```bash
git clone https://github.com/angelhuayas/acortador-enlaces.git
cd acortador-enlaces
```

## Ejecución

```bash
go run ./cmd/api
```

El servidor queda disponible en `http://localhost:3000`. Para comprobarlo:

```bash
curl http://localhost:3000/health
```

## Pruebas

```bash
go test ./...
```
