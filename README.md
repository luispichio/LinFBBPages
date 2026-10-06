# LinFBBPages

Aplicación web para la gestión de una instancia de [LinFBB](https://sourceforge.net/projects/linfbb/) (BBS de packet radio) que corre en el mismo host, o en un contenedor con el filesystem de FBB compartido.

## Características

### Etapa 1 (actual)

- **Login**: validación de callsign + contraseña contra `inf.sys` de FBB (solo usuarios con contraseña definida).
- **Mensajes**: listado paginado construido desde el índice `dirmes.sys`, lectura de cuerpos desde `mail/`, y redacción/envío de mensajes vía `mail/mail.in` (FBB los importa automáticamente en ~1 minuto).
- **Archivos 7+**: galería con previsualización de los archivos decodificados en `7pfbb/ok/`.

### Etapa 2 (futuro)

- Información de la aplicación
    - Referencias al proyecto LinFBB
    - Versión
    - Licencia
    - Repositorio de la aplicación
- Información del BBS
- Lista de mensajes
    - Ordenar por columnas
    - Scroll infinito
- Mensajería
    - Responder, responder por privado, reenviar, etc
    - Envío de archivos (conversión a 7+)
- Historial de mensajes leídos + Filtrado por "no leídos"
- 7+
    - Descargar
    - Abrir haciendo clic en la imágen
    - ...
- Terminal
    - Aplicación Web <-> Backend <-> Telnet <-> LinFBB (?)
- API

### Etapa 3 (futuro)

- Gestión de la instancia FBB
    - SysOp
        - Configuración
        - ADB de usuarios
        - logs
        - ...

## Arquitectura

- **Backend**: Go usando únicamente la biblioteca estándar. Produce un binario único y estático, pensado para hardware modesto (1 GHz de CPU / 512 MB de RAM).
- **Frontend**: HTML/CSS/JS vanilla, sin build step, embebido en el binario con `go:embed`. Interfaz multi-idioma (ES/EN).
- **Acceso a los datos de FBB**: lectura directa (y de solo lectura) de los archivos de FBB. La única escritura que realiza la aplicación sobre el directorio de FBB es crear/agregar a `mail/mail.in` para el envío de mensajes.

La galería detecta el MIME real de cada archivo. Esto permite previsualizar tanto imágenes `.jpg` como payloads JPEG que algunas instalaciones guardan con extensión `.7mf`; los `.7ix` y `.err` se muestran como archivos auxiliares.

```
┌─────────────┐   HTTP/JSON   ┌──────────────────┐   read   ┌─────────────────────┐
│  Frontend   │ ◄───────────► │  Backend (Go)    │ ───────► │ inf.sys, dirmes.sys │
│  (estático, │               │  net/http stdlib │          │ mail/*, 7pfbb/ok/*  │
│   embebido) │               │                  │ ───────► │ mail/mail.in        │
└─────────────┘               └──────────────────┘  append  └─────────────────────┘
```

## Requisitos

- Go 1.22+ (solo para compilar; `CGO_ENABLED=0` genera un binario sin dependencias).
- Acceso de lectura al directorio de datos de FBB (`/usr/local/var/ax25/fbb` por defecto) y de escritura sobre `mail/` para el envío de mensajes.

## Compilación y ejecución

```sh
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o linfbbpages ./cmd/linfbbpages
./linfbbpages --fbb-dir /usr/local/var/ax25/fbb --listen :8080
```

## Configuración

Todas las opciones pueden pasarse como flag o como variable de entorno.

| Flag            | Variable de entorno        | Default                     | Descripción |
|-----------------|----------------------------|-----------------------------|-------------|
| `--fbb-dir`     | `LINFBBPAGES_FBB_DIR`      | `/usr/local/var/ax25/fbb`   | Directorio de datos de FBB. |
| `--listen`      | `LINFBBPAGES_LISTEN`       | `:8080`                     | Dirección y puerto de escucha HTTP. |
| `--fbb-arch`    | `LINFBBPAGES_FBB_ARCH`     | `auto`                      | Formato binario de `inf.sys`/`dirmes.sys`: `auto`, `32` o `64` (ver abajo). |
| `--session-ttl` | `LINFBBPAGES_SESSION_TTL`  | `8h`                        | Duración de la sesión de login. |

### Formato binario de FBB (`--fbb-arch`)

Los archivos binarios de FBB (`inf.sys`, `dirmes.sys`) contienen structs de C con campos `long`, cuyo tamaño depende de cómo fue compilado FBB:

- **32 bits** (`long` = 4 bytes): registros de 360 bytes (`inf.sys`) y 194 bytes (`dirmes.sys`).
- **64 bits** (`long` = 8 bytes, con alineación a 8): registros de 384 bytes y 224 bytes.

Con `--fbb-arch=auto` (default) la aplicación detecta el formato al iniciar por divisibilidad del tamaño de los archivos. Para diagnosticar una instalación manualmente:

```sh
# inf.sys: size % 360 == 0 → 32 bits ; size % 384 == 0 → 64 bits
# dirmes.sys: size % 194 == 0 → 32 bits ; size % 224 == 0 → 64 bits
stat -c '%s %n' /usr/local/var/ax25/fbb/inf.sys /usr/local/var/ax25/fbb/dirmes.sys
```

Si ambos formatos son compatibles con el tamaño (archivos muy grandes, múltiplos de ambos registros), `auto` asume 64 bits. Se asume little-endian (válido en x86 y ARM habituales).

## Desarrollo

El directorio [`design/`](design/) contiene la documentación original de formatos de FBB (`design/docs/`) y **fixtures reales** de una instalación (`design/usr/local/var/ax25/fbb/`). Para correr en modo desarrollo contra los fixtures:

```sh
# Los fixtures están en formato 32 bits:
CGO_ENABLED=0 go run ./cmd/linfbbpages --fbb-dir design/usr/local/var/ax25/fbb --fbb-arch 32
```

```sh
go test ./...
```

## Notas de seguridad

- Las contraseñas en `inf.sys` están almacenadas en texto plano por FBB; la aplicación solo las lee para validar el login.
- Las sesiones son en memoria (se pierden al reiniciar el backend) y se identifican con una cookie httpOnly.
- Pensada para desplegarse en redes confiables (LAN/VPN). Si se expone a internet, colocarla detrás de un reverse proxy con TLS.

## Estructura del repositorio

```
├── cmd/linfbbpages/   # punto de entrada del binario
├── internal/
│   ├── fbb/           # parsers de inf.sys, dirmes.sys, mail/, 7pfbb/
│   ├── api/           # handlers HTTP, autenticación y sesiones
│   └── config/        # configuración (flags + env)
├── web/static/        # frontend embebido (index.html, app.js, style.css, i18n/)
└── design/            # documentación de formatos FBB + fixtures para tests/dev
```

## Licencia

A definir.
