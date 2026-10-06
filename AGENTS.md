# AGENTS.md — LinFBBPages

Contexto para agentes que trabajen en este repositorio. Leer completo antes de modificar código.

## Qué es este proyecto

Aplicación web para gestionar una instancia de [LinFBB](https://sourceforge.net/projects/linfbb/) (BBS de packet radio) que corre en el mismo host (o contenedor con filesystem compartido). Etapa 1: login, visualización/redacción de mensajes y visualización de archivos decodificados 7+. Etapa 2 (futura): gestión de la instancia.

El backend **lee directamente los archivos binarios de datos de FBB**. La corrección de los parsers es lo más crítico del proyecto.

## Stack y restricciones

- **Backend**: Go, **solo biblioteca estándar** (`net/http`, `encoding/binary`, `embed`, `crypto/rand`, ...). No agregar dependencias externas sin aprobación explícita del usuario. Target: hardware modesto (1 GHz / 512 MB RAM).
- **Frontend**: HTML/CSS/JS vanilla en `web/static/`, **sin build step ni npm**, embebido con `go:embed`. i18n ES/EN mediante diccionarios JS (`web/static/i18n/es.js`, `en.js`); idioma default ES, detección por `navigator.language`.
- **Sesiones**: en memoria (mapa token→usuario con expiración), cookie httpOnly `SameSite=Lax`, token de 32 bytes de `crypto/rand`. No persistir sesiones en disco.

## Estructura del repositorio

```
├── cmd/linfbbpages/   # main.go: config, wiring, servidor HTTP
├── internal/
│   ├── fbb/           # parsers: inf.go, dirmes.go, mail.go, sevenplus.go (+ _test.go)
│   ├── api/           # handlers HTTP, auth, sesiones
│   └── config/        # flags + variables de entorno
├── web/static/        # frontend embebido
└── design/            # NO TOCAR: docs de formatos FBB + fixtures para tests/dev
```

## Comandos

```sh
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o linfbbpages ./cmd/linfbbpages # compilar
go test ./...                                # tests
go vet ./...                                 # lint básico

# Dev contra fixtures (¡están en formato 32 bits!):
CGO_ENABLED=0 go run ./cmd/linfbbpages --fbb-dir design/usr/local/var/ax25/fbb --fbb-arch 32
```

## Formatos de archivos de FBB

Documentación original en `design/docs/` (`fmtinf.html`, `fmtdirme.html`, `fmtmail.html`). Los fixtures reales están en `design/usr/local/var/ax25/fbb/`.

### Reglas generales

- Strings de C: terminar en el primer byte NUL (`00`); descartar el resto del campo.
- Enteros: **little-endian**. Fechas: segundos desde 1970-01-01 00:00 UTC (Unix epoch).
- `long` de C = 4 bytes en builds de 32 bits, 8 bytes en builds de 64 bits (con alineación a 8 → padding). El layout se selecciona con `--fbb-arch` (`auto`|`32`|`64`, default `auto`) y los parsers deben ser **table-driven** a partir de un tipo `Layout`: prohibido hardcodear offsets fuera de la definición del layout.
- Los archivos pueden cambiar mientras FBB corre: cachear parseos por **mtime** y releer al cambiar. Nunca mantener handles abiertos.
- Nota histórica: la doc declara `flags`/`on_base` como `unsigned`, pero el fixture valida que ocupan **2 bytes** (2880 = 8×360 y `pass` encontrado en offset 338).

### `inf.sys` — usuarios (login)

Registros de tamaño fijo, **sin header**. Tamaño del archivo % tamaño de registro == 0. Login = callsign (case-insensitive) con `pass` no vacío que coincida.

| Campo | Tamaño | Offset (32 bits, reg=360) | Offset (64 bits, reg=384) |
|---|---|---|---|
| `indic` (callsign[7]+ssid) | 8 | 0 | 0 |
| `relai[8]` (path digis) | 64 | 8 | 8 |
| `lastmes` | long | 72 | 72 |
| `nbcon` | long | 76 | 80 |
| `hcon` | long | 80 | 88 |
| `lastyap` | long | 84 | 96 |
| `flags` | 2 | 88 | 104 |
| `on_base` | 2 | 90 | 106 |
| `nbl` | 1 | 92 | 108 |
| `lang` | 1 | 93 | 109 |
| _(padding)_ | — | — | 110–111 |
| `newbanner` | long | 94 | 112 |
| `download` | 2 | 98 | 120 |
| `free[20]` | 20 | 100 | 122 |
| `thema` | 1 | 120 | 142 |
| `nom[18]` | 18 | 121 | 143 |
| `prenom[13]` | 13 | 139 | 161 |
| `adres[61]` | 61 | 152 | 174 |
| `ville[31]` | 31 | 213 | 235 |
| `teld[13]` | 13 | 244 | 266 |
| `telp[13]` | 13 | 257 | 279 |
| `home[41]` (home BBS) | 41 | 270 | 292 |
| `qra[7]` (QTH locator) | 7 | 311 | 333 |
| `priv[13]` | 13 | 318 | 340 |
| `filtre[7]` | 7 | 325 | 353 |
| **`pass[13]`** | 13 | **338** | **360** |
| `zip[9]` | 9 | 351 | 373 |
| **Total registro** | | **360** | **384** (382 + 2 tail pad) |

### `dirmes.sys` — índice de mensajes

Registros de tamaño fijo. **El registro 0 es un header**: solo es válido su campo `numero` (último número de mensaje asignado). Los mensajes son los registros 1..N; un `type` NUL (`00`) invalida el registro.

| Campo | Tamaño | Offset (32 bits, reg=194) | Offset (64 bits, reg=224) |
|---|---|---|---|
| `type` (A,B,P,T) | 1 | 0 | 0 |
| `status` ($,A,F,K,N,Y) | 1 | 1 | 1 |
| _(padding)_ | — | — | 2–7 |
| `numero` | long | 2 | 8 |
| `taille` (tamaño del cuerpo) | long | 6 | 16 |
| `date` | long | 10 | 24 |
| `bbsf[7]` (BBS que lo entregó) | 7 | 14 | 32 |
| `bbsv[41]` (ruta) | 41 | 21 | 39 |
| `exped[7]` (origen/from) | 7 | 62 | 80 |
| `desti[7]` (destino/to) | 7 | 69 | 87 |
| `bid[13]` (BID/MID) | 13 | 76 | 94 |
| `titre[61]` (título) | 61 | 89 | 107 |
| `free[16]` | 16 | 150 | 168 |
| `datesd` (creación) | long | 166 | 184 |
| `datech` (último cambio status) | long | 170 | 192 |
| `fbbs[10]` (máscara BBS a forwardear) | 10 | 174 | 200 |
| `forw[10]` (máscara ya forwardeado) | 10 | 184 | 210 |
| **Total registro** | | **194** | **224** (220 + 4 tail pad) |

Tipos: `B` bulletin, `P` privado, `A`/`T` menos comunes. Status: `N` nuevo, `Y` leído, `F` forwardeado, `K` killed, `A` archivado, `$` en proceso. Detalle semántico completo en `design/docs/fmtdirme.html`.

### Detección de arquitectura (`--fbb-arch=auto`)

Al iniciar, para `inf.sys` y `dirmes.sys`:

1. Candidatos = {32, 64}; descartar el arch cuyo tamaño de registro **no** divida exactamente al tamaño del archivo (intersección de ambos archivos).
2. Queda un candidato → ese.
3. Quedan ambos (tamaño múltiplo de ambos registros) → asumir **64** y loguear la ambigüedad.
4. No queda ninguno → error fatal con mensaje claro (archivo corrupto o formato desconocido).

### Cuerpos de mensajes — `mail/`

- Archivo por mensaje: `mail/mail<N>/m_%06d.mes` donde **N = numero % 10** (validado: `m_000110` está en `mail0`).
- Contenido texto: cero o más líneas de header de ruteo `R:AAMMDD/hhmmZ ...`, luego línea en blanco, luego el cuerpo. Parsear los `R:` como headers y el resto como texto plano.
- `mail/mail.in` puede existir (cola de importación de FBB): **no exponerlo como mensaje**.

### Envío de mensajes — `mail/mail.in`

FBB chequea `mail/mail.in` cada minuto, importa los mensajes y **borra el archivo**. La app lo crea/agrega (con lock de archivo) con uno o más mensajes en formato:

```
SP <destino>[@ruta] < <origen> $<BID-opcional>
<título>
<cuerpo...>
/EX
```

`SP` = privado, `SB` = bulletin. Ejemplos reales en el fixture `design/usr/local/var/ax25/fbb/mail/mail.in` y doc en `design/docs/fmtmail.html`. La UI debe comunicar el delay de ~1 minuto.

### Archivos decodificados 7+ — `7pfbb/ok/`

- Archivos útiles (ej. `.jpg`) junto a metadatos por basename: `.7ix` (índice) y `.err` (reporte de errores 7PLUS). En instalaciones reales, `.7mf` puede contener directamente el payload decodificado (por ejemplo, un JPEG), por lo que se detecta por MIME y se muestra como archivo principal.
- La vista los agrupa por basename: mostrar el archivo principal con preview si es imagen; los metadatos como metadata opcional, nunca mezclados en la galería.
- `7pfbb/7pl_log` y las partes crudas `*.pNN` en `7pfbb/` **no** se exponen en etapa 1.

## API HTTP

JSON, cookie de sesión en todos menos `/api/login`. Auth middleware rechaza con 401.

| Método | Ruta | Descripción |
|---|---|---|
| POST | `/api/login` | `{callsign, password}` → valida contra `inf.sys`, setea cookie. |
| POST | `/api/logout` | Invalida la sesión. |
| GET | `/api/me` | Datos del usuario logueado (callsign, nombre, QTH). |
| GET | `/api/messages` | Lista paginada desde `dirmes.sys`. Query: `type`, `q` (título), `page`, `page_size` (default 50). Orden: número descendente. |
| GET | `/api/messages/{num}` | Metadata + headers `R:` + cuerpo desde `mail/`. 404 si no existe. |
| POST | `/api/messages` | Redactar: `{to, route, type: P|B, title, body}` → append a `mail.in`. Respuesta inmediata, importación async (~1 min). |
| GET | `/api/files` | Archivos de `7pfbb/ok/` agrupados por basename (principal + auxiliares). |
| GET | `/api/files/{name}` | Sirve un archivo (inline si imagen). **Sanitizar contra path traversal.** |
| GET | `/*` | Frontend estático embebido. |

## Reglas de seguridad e invariantes (no negociables)

1. **Jamás escribir ni modificar archivos de FBB** (`inf.sys`, `dirmes.sys`, `mail/*.mes`, `7pfbb/**`). Única excepción: crear/agregar `mail/mail.in`.
2. Los fixtures de `design/` son **read-only**: no editarlos ni borrarlos; los tests no deben mutarlos.
3. Todo endpoint que reciba un nombre de archivo debe validarlo (sin `..`, sin separadores de ruta) antes de abrir nada.
4. Login solo para callsigns con password no vacío; comparación sensible a mayúsculas para el password, insensible para el callsign.
5. Los mensajes con status inválido/type NUL no se muestran.

## Convenciones

- Cambios mínimos y focalizados; seguir el estilo del código existente.
- Todo parser nuevo va con tests: layout 32 contra fixtures reales de `design/`, layout 64 con registros sintéticos construidos según las tablas de arriba.
- Comentarios y UI en español salvo identificadores de código (inglés).
- Si se modifica algo documentado aquí (formatos, endpoints, estructura, comandos), **actualizar este AGENTS.md y el README.md en el mismo cambio**.
