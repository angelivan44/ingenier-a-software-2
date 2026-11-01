# Acortador de enlaces

Servicio web en Go para crear enlaces cortos, redirigir a la URL original y consultar cuántos clics recibió cada enlace.

Proyecto del curso Ingeniería de Software 2, Universidad Continental.

## Requisitos

- Go 1.25 o superior
- Postgres, opcional en desarrollo

## Instalación

```bash
git clone https://github.com/angelivan44/ingenier-a-software-2.git
cd ingenier-a-software-2
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
{"code":"aZ3kP9q","url":"https://example.com/articulo","short_url":"http://localhost:3000/aZ3kP9q","clicks":0,"created_at":"2026-11-01T19:00:00Z"}
```

Si la URL no es `http` o `https`, o no incluye dominio, responde `400 Bad Request`.

Abrir un enlace corto redirige a la URL original (`302 Found`). Si el código no existe, responde `404 Not Found`:

```bash
curl -i http://localhost:3000/aZ3kP9q
```

Consultar los datos y los clics de un enlace. Esta consulta no suma clics, y tampoco las peticiones `HEAD`. Si el código no existe, responde `404 Not Found`:

```bash
curl http://localhost:3000/api/links/aZ3kP9q
```

## Almacenamiento

Si la variable `DATABASE_URL` está definida, los enlaces se guardan en Postgres y la tabla se crea al iniciar. Si no lo está, se guardan en memoria y se pierden al reiniciar el servidor.

```bash
DATABASE_URL="postgres://usuario:clave@localhost:5432/acortador" go run ./cmd/api
```

## Pruebas

```bash
go test ./...
```

Las pruebas de Postgres se omiten si no se define `TEST_DATABASE_URL`. Usa una base de pruebas, no la de producción:

```bash
TEST_DATABASE_URL="postgres://usuario:clave@localhost:5432/acortador_test" go test ./...
```

## Integración continua

Cada push a `main` o `develop` ejecuta el workflow `.github/workflows/ci.yml`, que revisa el formato, analiza el código con `go vet` y `golangci-lint`, corre las pruebas con el detector de condiciones de carrera y compila el servidor.
