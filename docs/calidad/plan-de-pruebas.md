# Plan de pruebas

## Objetivo

Comprobar que el acortador crea, redirige, cuenta, vence y elimina enlaces según las reglas de la API, y que rechaza las entradas inválidas sin exponer errores internos.

## Alcance

| Se prueba | Por qué | Nivel |
|---|---|---|
| Validación de URL | Es la entrada principal del usuario y la fuente de varios defectos | Unitario y API |
| Alias personalizado | Tiene reglas de largo, caracteres y rutas reservadas | Unitario y API |
| Expiración | Depende de fechas y límites, propensa a errores de un instante | Unitario y API |
| Eliminación con token | Es una operación destructiva y debe exigir el token correcto | Unitario y API |
| Redirección y clics | Es el uso principal; no debe perder clics ni contar los que no son visitas | Unitario, API y Postgres |
| Almacenamiento en Postgres | Es el almacenamiento de producción | Integración |
| Respuestas de error | Los clientes esperan JSON y no deben ver errores internos | API |

No se prueba: el rendimiento bajo carga, la interfaz en distintos navegadores ni la infraestructura de Vercel y Neon.

## Herramientas

- `go test` (paquete `testing` de Go) para pruebas unitarias y de API.
- `net/http/httptest` para llamar a los manejadores HTTP sin levantar el servidor.
- Postgres 17 en un contenedor de servicio de GitHub Actions para las pruebas de integración.
- `go test -race` para detectar condiciones de carrera en los contadores de clics.
- `go tool cover` para la cobertura.

## Técnicas

- **Caja negra:** particiones de equivalencia (URL válida o inválida), valores límite (largo del alias, días de expiración, instante de vencimiento) y tabla de decisión (cabecera y token al eliminar).
- **Caja blanca:** cobertura de ramas y condiciones del código: reintentos de `Create`, cada condición de `Delete`, las ramas de `pointsTo` y la comparación `!Before` de `Expired`. Se usan dobles de prueba (`fakeStore`, `failingStore`) para forzar errores del almacenamiento y un reloj fijo para la expiración.

## Criterios de aceptación

1. Todas las pruebas automatizadas pasan en el pipeline, incluidas las de Postgres.
2. La cobertura total de sentencias es de al menos 70 %. El pipeline falla si es menor.
3. `go vet` y `golangci-lint` no reportan problemas.
4. Ningún defecto de severidad alta o media queda abierto al entregar.
5. Ninguna respuesta de error expone mensajes internos de la base de datos.

## Casos de prueba

"Obtenido" es el resultado de la última ejecución. En los casos que detectaron un defecto se indica también lo obtenido antes de la corrección.

| ID | Técnica | Descripción | Entrada | Resultado esperado | Resultado obtenido | Prueba automatizada |
|---|---|---|---|---|---|---|
| CP-01 | Caja negra: particiones | URL válida se acorta | `https://example.com/ruta?a=1` con espacios alrededor | Enlace con código de 7 letras o números, URL sin espacios | Cumple | `TestCreateGuardaEnlaceConCodigoValido` |
| CP-02 | Caja negra: particiones | URLs inválidas se rechazan | vacía, `example.com`, `ftp://…`, `javascript:…`, `https://`, `http://:8080` | `ErrInvalidURL` / 400 | Cumple | `TestCreateRechazaURLInvalida` |
| CP-03 | Caja negra: particiones | URL con credenciales se rechaza | `https://usuario:clave@example.com` | 400 | Antes: se aceptaba (defecto D-01). Después: cumple | `TestCreateRechazaURLInvalida` |
| CP-04 | Caja negra: valores límite | Largo del alias | 2, 3, 30 y 31 caracteres | 2 y 31 rechazados; 3 y 30 aceptados | Cumple | `TestCreateAliasLimitesDeLargo` |
| CP-05 | Caja negra: particiones | Alias con caracteres o nombres no permitidos | `mi promo`, `mi/promo`, `promoción`, `mi.promo`, `api`, `HEALTH` | `ErrInvalidAlias` / 400 | Cumple | `TestCreateRechazaAliasInvalido` |
| CP-06 | Caja negra | Alias ocupado | Dos creaciones con alias `promo` | Segunda responde 409 y el enlace original no cambia | Cumple | `TestCreateLinkRespondeConflictoSiElAliasExiste` |
| CP-07 | Caja negra: valores límite | Plazo de expiración | -1, 0, 1, 365 y 366 días | -1 y 366 rechazados; 0 no vence; 1 y 365 vencen en esa fecha | Cumple | `TestCreateExpiracionLimites` |
| CP-08 | Caja blanca: condición | Instante exacto de vencimiento | 1 ns antes, en el instante y 1 ns después de `ExpiresAt` | No vencido, vencido, vencido | Cumple | `TestLinkExpiredEnElLimite` |
| CP-09 | Caja negra | Enlace vencido | Visita tras 1 día con plazo de 1 día | 410 Gone en GET y HEAD, la visita no suma clic, la consulta sigue disponible | Cumple | `TestVisitRechazaEnlaceVencidoSinContarClic`, `TestRedirectRespondeGoneSiElEnlaceVencio` |
| CP-10 | Caja negra: tabla de decisión | Eliminar según cabecera y token | Sin cabecera, sin `Bearer`, `Bearer` vacío, token incorrecto, código inexistente, token correcto | 401, 401, 401, 403, 404, 204 | Cumple | `TestDeleteLinkRespuestasDeError`, `TestDeleteLinkConTokenCorrecto` |
| CP-11 | Caja blanca: condiciones | Cada condición que rechaza el token | Token vacío, incorrecto, de otro enlace, el hash guardado; enlace antiguo sin hash | `ErrForbidden` en todos y el enlace no se borra | Cumple | `TestDeleteRechazaTokenInvalido`, `TestDeleteRechazaEnlacesSinToken` |
| CP-12 | Caja blanca: ramas | Reintentos al generar código | Almacenamiento que choca 2 veces, 5 veces, y que falla por otro error | 3 intentos y éxito; 5 intentos y error; 1 intento y error original | Cumple | `TestCreateReintentaSiElCodigoExiste`, `TestCreateFallaAlAgotarIntentos`, `TestCreateNoReintentaOtrosErrores` |
| CP-13 | Caja blanca: ramas | Enlace al propio acortador | Mismo dominio, con puerto, en mayúsculas, subdominio, sin host conocido | Rechaza mismo dominio con o sin puerto y mayúsculas; acepta subdominio y otros | Antes: se aceptaban (defecto D-02). Después: cumple | `TestCreateRechazaURLDelPropioAcortador`, `TestCreateLinkRechazaURLDelPropioAcortador` |
| CP-14 | Caja negra | Datos después del JSON | `{"url":"…"} basura`, dos objetos seguidos, JSON con salto de línea final | 400, 400, 201 | Antes: 201 en los dos primeros (defecto D-03). Después: cumple | `TestCreateLinkRechazaDatosDespuesDelJSON` |
| CP-15 | Caja negra | Rutas y métodos desconocidos | `GET /abc1234/`, `GET /api/links`, `PUT /api/links/abc1234`, `POST /abc1234` | 404 o 405 en JSON con campo `error` y cabecera `Allow` | Antes: texto plano (defecto D-04). Después: cumple | `TestRutasDesconocidasRespondenJSON` |
| CP-16 | Caja negra | Clics simultáneos | 100 visitas concurrentes en memoria y 50 en Postgres | El contador termina exactamente en 100 y 50 | Cumple, también con `-race` | `TestVisitNoPierdeClicsSimultaneos`, `TestPostgresNoPierdeClicsSimultaneos` |
| CP-17 | Caja blanca: ramas de error | Falla de la base de datos | Almacenamiento que siempre falla | 500 con mensaje genérico, sin el texto del error interno | Cumple | `TestCreateLinkNoExponeErroresInternos`, `TestStatsNoExponeErroresInternos`, `TestRedirectNoExponeErroresInternos`, `TestDeleteLinkNoExponeErroresInternos` |

## Ejecución

```bash
go test -race -cover ./internal/...
```

Con un Postgres de prueba en Docker:

```bash
docker run -d --rm --name pg-pruebas -p 5432:5432 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=acortador_test postgres:17
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/acortador_test?sslmode=disable" go test -race -cover ./internal/...
```

El pipeline `.github/workflows/ci.yml` ejecuta lo mismo en cada push y pull request a `develop` y `main`.
