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

## Uso

Crear un enlace corto:

```bash
curl -X POST http://localhost:3000/api/links \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/articulo"}'
```

Respuesta (`201 Created`):

```json
{"code":"aZ3kP9q","url":"https://example.com/articulo","short_url":"http://localhost:3000/aZ3kP9q"}
```

Si la URL no es `http` o `https`, o no incluye dominio, responde `400 Bad Request`.

Por ahora los enlaces se guardan en memoria y se pierden al reiniciar el servidor.

## Pruebas

```bash
go test ./...
```
