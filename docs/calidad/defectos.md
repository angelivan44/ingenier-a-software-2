# Registro de defectos

Cada defecto se registra como Issue en GitHub con la plantilla `.github/ISSUE_TEMPLATE/defecto.md` y se corrige en una rama `fix/`. La rama tiene dos commits: una prueba que reproduce el defecto y falla, y la corrección que la hace pasar. El pull request cierra el Issue con `Closes #N`.

Severidad:

- **Alta:** expone datos o permite un uso indebido.
- **Media:** produce un comportamiento incorrecto visible para el usuario.
- **Baja:** inconsistencia que no rompe el uso normal.

| ID | Título | Severidad | Rama | Estado |
|---|---|---|---|---|
| D-01 | Se aceptan URLs con usuario y contraseña | Alta | `fix/url-con-credenciales` | Resuelto |
| D-02 | Se aceptan enlaces que apuntan al propio acortador | Media | `fix/enlace-al-propio-acortador` | Resuelto |
| D-03 | Se aceptan datos después del JSON | Baja | `fix/json-con-datos-extra` | Resuelto |
| D-04 | Rutas y métodos desconocidos responden texto plano | Baja | `fix/errores-en-json` | Resuelto |

---

## D-01 · Se aceptan URLs con usuario y contraseña

**Descripción:** `POST /api/links` acepta URLs con credenciales, como `https://usuario:clave@example.com`. La URL de destino se muestra a cualquiera en `GET /api/links/{code}`, así que las credenciales quedan públicas.

**Pasos para reproducir:**

1. `curl -X POST http://localhost:3000/api/links -d '{"url":"https://usuario:clave@example.com"}'`
2. `curl http://localhost:3000/api/links/<code>`

**Resultado esperado:** `400 Bad Request` en el paso 1.

**Resultado obtenido:** `201 Created`, y el paso 2 muestra `usuario:clave`.

**Severidad:** Alta. Expone credenciales a cualquier persona que conozca el código.

**Prueba que lo detecta:** `TestCreateRechazaURLInvalida`, casos "con usuario y contraseña" y "con usuario" (`internal/links/links_test.go`).

```
--- FAIL: TestCreateRechazaURLInvalida/con_usuario_y_contraseña
    links_test.go:172: error = <nil>, se esperaba ErrInvalidURL
--- FAIL: TestCreateRechazaURLInvalida/con_usuario
    links_test.go:172: error = <nil>, se esperaba ErrInvalidURL
```

**Corrección:** `validateURL` rechaza las URLs con `User` definido.

---

## D-02 · Se aceptan enlaces que apuntan al propio acortador

**Descripción:** Se puede acortar una URL del mismo dominio del acortador, como otro enlace corto. Así se forman redirecciones en cadena, y dos alias que se apuntan entre sí forman un bucle que el navegador corta con un error.

**Pasos para reproducir:**

1. Crear un enlace y anotar su `short_url`.
2. `curl -X POST http://localhost:3000/api/links -d '{"url":"<short_url del paso 1>"}'`

**Resultado esperado:** `400 Bad Request`.

**Resultado obtenido:** `201 Created`. Lo mismo con el dominio en mayúsculas o con el puerto explícito.

**Severidad:** Media. El usuario final ve un error del navegador y los clics se cuentan en enlaces que no son el destino real.

**Prueba que lo detecta:** `TestCreateLinkRechazaURLDelPropioAcortador` (`internal/server/server_test.go`).

```
--- FAIL: TestCreateLinkRechazaURLDelPropioAcortador
    server_test.go:453: http://example.com/b9Iepq2: código = 201, se esperaba 400
    server_test.go:453: https://example.com/b9Iepq2: código = 201, se esperaba 400
    server_test.go:453: http://EXAMPLE.com/otro: código = 201, se esperaba 400
    server_test.go:453: http://example.com:80/otro: código = 201, se esperaba 400
```

**Corrección:** el servidor envía su dominio en `NewLink.ServiceHost`, y `Create` rechaza con `ErrSelfLink` una URL hacia ese dominio, sin importar el puerto ni las mayúsculas.

---

## D-03 · Se aceptan datos después del JSON

**Descripción:** `POST /api/links` lee solo el primer objeto JSON del cuerpo e ignora lo que viene después. Una petición mal formada se acepta en silencio y el cliente no se entera de su error.

**Pasos para reproducir:**

1. `curl -X POST http://localhost:3000/api/links -d '{"url":"https://example.com"} basura'`

**Resultado esperado:** `400 Bad Request`.

**Resultado obtenido:** `201 Created`.

**Severidad:** Baja. No rompe el uso normal, pero oculta errores de los clientes.

**Prueba que lo detecta:** `TestCreateLinkRechazaDatosDespuesDelJSON` (`internal/server/server_test.go`).

```
--- FAIL: TestCreateLinkRechazaDatosDespuesDelJSON
    server_test.go:481: {"url":"https://example.com"} basura: código = 201, se esperaba 400
    server_test.go:481: {"url":"https://example.com"}{"url":"https://otro.com"}: código = 201, se esperaba 400
```

**Corrección:** `decodeJSON` exige el fin del cuerpo después del objeto. Se siguen permitiendo espacios y saltos de línea al final.

---

## D-04 · Rutas y métodos desconocidos responden texto plano

**Descripción:** Las rutas que no existen y los métodos no permitidos responden el texto plano del enrutador (`404 page not found`, `Method Not Allowed`). El resto de la API responde JSON con el campo `error`, y la página principal lee la respuesta como JSON, así que muestra un error de lectura en vez del mensaje.

**Pasos para reproducir:**

1. `curl -i http://localhost:3000/abc1234/`
2. `curl -i -X PUT http://localhost:3000/api/links/abc1234`

**Resultado esperado:** `404` y `405` con `Content-Type: application/json` y cuerpo `{"error":"..."}`.

**Resultado obtenido:** `Content-Type: text/plain` y cuerpo `404 page not found` o `Method Not Allowed`.

**Severidad:** Baja. Afecta solo a peticiones incorrectas, pero rompe el contrato de la API.

**Prueba que lo detecta:** `TestRutasDesconocidasRespondenJSON` (`internal/server/server_test.go`).

```
--- FAIL: TestRutasDesconocidasRespondenJSON/GET_/abc1234/
    server_test.go:516: Content-Type = "text/plain; charset=utf-8", se esperaba application/json
    server_test.go:520: cuerpo = "404 page not found\n", se esperaba JSON con el campo error
```

**Corrección:** `jsonErrors` envuelve el enrutador. Cuando ninguna ruta coincide, responde el mismo código en JSON y conserva la cabecera `Allow`.
