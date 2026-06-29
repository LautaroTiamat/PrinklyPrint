# Changelog

Todas las versiones notables de PrinklyPrint quedan documentadas acá. Formato basado en [Keep a Changelog](https://keepachangelog.com/es/1.1.0/) y el proyecto sigue [Semantic Versioning](https://semver.org/lang/es/).

## [1.2.0] — 2026-06-29

Endurecimiento de seguridad en dos frentes: la **cadena de suministro** del pipeline de build/release (con evidencia auditable) y el **agente** en sí, cerrando tres brechas del informe — datos en reposo sin cifrar, validación de entrada incompleta y logging no integrado a SIEM.

### Agregado
- **Escaneos de seguridad continuos** ([`.github/workflows/security.yml`](.github/workflows/security.yml), en push/PR a main + semanal): **govulncheck** (SCA), **gosec** (SAST → SARIF), **CodeQL** (SAST, `build-mode: none`) y **gitleaks** (secretos).
- **SBOM CycloneDX** (`bom.json`, generado con `cyclonedx-gomod`) adjuntado a cada release.
- **Firma Authenticode** del `.exe` y del instalador, con timestamp RFC3161 y parametrizable (`.pfx` en secret o servicio externo/HSM). Ver [`.github/scripts/sign.ps1`](.github/scripts/sign.ps1). La verificación post-firma clasifica por `Get-AuthenticodeSignature.Status`: **corta el build** ante `NotSigned` y `HashMismatch` (binario alterado tras firmar = integridad rota), pero **solo advierte** ante `NotTrusted`/cadena no confiable en el runner (esperado con una CA interna). Cubierto por [`sign.Tests.ps1`](.github/scripts/sign.Tests.ps1).
- **Attestation de build provenance** (`actions/attest-build-provenance`) del `.exe` y el instalador.
- **[Dependabot](.github/dependabot.yml)** para `github-actions`, `gomod` y `docker` (semanal).
- **[`checksums.txt`](checksums.txt)** con el SHA256 de SumatraPDF; el build verifica el binario descargado y **falla** si no coincide.
- **[`tools.go`](tools.go)** que fija `rsrc` en `go.mod`/`go.sum` (integridad lockeada + Dependabot).
- **Cifrado de PDFs en reposo** con DPAPI de Windows (`CryptProtectData`, scope de USUARIO: un blob exfiltrado a otro usuario/equipo no se puede descifrar). Los PDFs se guardan como `<id>.pdf.enc` (blobs cifrados, no PDFs legibles) y se descifran a un archivo temporal con ACL restrictiva **solo** para imprimir, que se borra (overwrite best-effort + remove) apenas SumatraPDF termina. Nuevo paquete `internal/crypto/dpapi` (`_windows`/`_stub`).
- **Permisos owner-only** vía DACL protegida en Windows (`internal/winfs`: SDDL `D:PAI(A;;FA;;;OW)(A;;FA;;;SY)(A;;FA;;;BA)` + `SetNamedSecurityInfo`). Aplicado a `auth.json`, `agent.db` (+ `-wal`/`-shm`), `config.yaml`, el dir de logs, el dir de PDFs y el dir temporal de impresión. En no-Windows (dev/CI): `0o600`/`0o700`.
- **Validación de entrada en `POST /print`**: magic bytes `%PDF-` (en `Enqueue`), tope de `copies` (≤999), enums cerrados de `orientation`/`duplex`/`scale`, `paper_size` con charset seguro, **whitelist estricta de `page_range`** (`^[0-9,\- ]*$`, evita inyectar directivas a `-print-settings` de SumatraPDF), tope de `metadata` (16 KB), bounds de paginación en `GET /jobs` (limit 1..500, offset ≥ 0) y profundidad máxima de cola (429 `queue_full` a 1000 jobs `queued`).
- **Eventos de seguridad a SIEM**: nuevo `internal/seclog` que emite a slog (archivo) **y** al Windows Event Log (canal Application, source `PrinklyPrint`) con IDs estables: `auth_failure` (1001), `pairing_approved` (1002), `pairing_denied` (1003), `print_enqueued` (1004), `settings_changed` (1005), `token_rotated` (1006). **Los 401 ahora se loguean** (`auth_failure`) — antes eran invisibles. Nunca se loguea el token. Registro del source: `prinklyprint --register-eventlog` (lo corre el instalador, elevado) / `--unregister-eventlog` en el desinstalador.

### Cambiado
- **Todas las GitHub Actions pineadas por commit SHA** (antes `@v4`/`@v5`/`@v2`, tags mutables).
- **Imagen base del Dockerfile pineada por digest** (`golang:1.25-alpine@sha256:…`), antes por tag.
- **rsrc** dejó de instalarse con `@latest`: ahora `go run` con integridad de `go.sum` (CI) / `@v0.10.2` (Docker).
- **Permisos del workflow con mínimo privilegio**: se quitó `contents: write` global; cada job pide lo justo (`contents: write` solo en release; `security-events: write` en SAST; `id-token`/`attestations: write` en provenance). `persist-credentials: false` en los checkouts.
- **Build reproducible**: se agregó `-ldflags "-buildid="` (ya estaban `-trimpath` y `CGO_ENABLED=0`).
- **Go 1.22 → 1.25**: la 1.22 quedó fuera de la ventana de parches y arrastraba 27 vulnerabilidades de la stdlib (request smuggling en net/http, etc.) que govulncheck marcaba. Con 1.25, govulncheck reporta **0 vulnerabilidades** en el código.
- Al arrancar, el worker **barre el dir temporal** (`print-tmp`): si una corrida anterior murió a mitad de impresión (crash/kill/corte de luz) y quedó un PDF descifrado en claro, se scrubbea y borra.
- Los eventos al Event Log **sanean CR/LF y caracteres de control** y acotan la longitud de los campos controlados por el cliente (filename, Origin), para evitar log-forging en el SIEM.
- `sumatra_log` (stdout/stderr de SumatraPDF en la DB) se trunca a 4 KB (UTF-8-safe): es data de debug que puede filtrar rutas/contenido.
- `config.yaml` y los logs pasan de `0o644` a `0o600`; el dir de logs y el de PDFs a `0o700`.
- Errores de input de `/print` ahora devuelven `400 {"error":"bad_request"}` de forma consistente.

### Eliminado
- **BREAKING — se eliminó `pdf_url` de `POST /print`.** El agente ya no descarga PDFs desde una URL remota: los PDF se generan en el cliente y se mandan **siempre** inline en `pdf_base64` (único camino). Se quitaron el campo `pdf_url` del request, la función de descarga (`downloadPDF`) y el cliente HTTP saliente del worker. Un body con `pdf_url` se ignora (campo desconocido) y, al faltar `pdf_base64`, devuelve `400 bad_request`. **Migración**: si alguna integración mandaba `pdf_url`, ahora debe descargar/generar el PDF de su lado y enviarlo en `pdf_base64`.

### Seguridad
- Cierra las brechas del informe de auditoría: integridad de pipeline, integridad de artefactos / OWASP A08 (firma + provenance + verificación de hash), SBOM, pruebas de seguridad, gestión de vulnerabilidades (govulncheck + Dependabot + bump de Go), **datos en reposo (cifrado DPAPI + ACL owner-only), validación de entrada y logging/SIEM**.
- **SSRF eliminado de raíz**: al quitar `pdf_url` (ver *Eliminado*), el agente no realiza ninguna conexión saliente de red. Además, como esa era la única rama que escribía PDFs en claro, el **cifrado en reposo de los PDFs queda completo** (ya no hay PDFs sin cifrar en disco).
- **Requiere configurar en Settings del repo** (no alcanza con los workflows): branch protection con required status checks, code scanning como required, y secret scanning + push protection. Ver README → *Seguridad de la cadena de suministro*.
- **Nuevos secrets para firmar** (opcionales para builds de prueba, obligatorios para el release de producción): `WINDOWS_CERT_PFX_BASE64` + `WINDOWS_CERT_PASSWORD`, o `SIGN_COMMAND`. Ver README.

### Documentación
- **Documentación de seguridad y operación** en [`docs/`](docs/), basada en el código real: modelo de amenazas ([`THREAT_MODEL.md`](docs/THREAT_MODEL.md)), guía de despliegue y hardening on-premise ([`DEPLOYMENT_HARDENING.md`](docs/DEPLOYMENT_HARDENING.md)), runbook operativo ([`RUNBOOK.md`](docs/RUNBOOK.md)) y guía de pruebas de seguridad con PoCs ([`PENTEST_GUIDE.md`](docs/PENTEST_GUIDE.md)).
- **[`SECURITY.md`](SECURITY.md)** con la política de divulgación responsable, versiones soportadas y el resumen de la postura de seguridad. README enlaza `docs/` y `SECURITY.md`.

### Tests
- **Cobertura de tests ampliada** sobre la lógica testeable en Linux/CI: traducción de opciones a flags de SumatraPDF con regresión de inyección en `page_range` (`internal/printer`), validadores y whitelists de `/print` (`internal/server`), validación y round-trip de config (`internal/config`), y operaciones de la cola — conteos, paginación y recovery de jobs `printing`→`queued` (`internal/store`). `go test ./...` en verde. Las capas Windows-only (DACL, DPAPI, Event Log, UI) quedan pendientes de validación en Windows real.

### Pendiente de validación en Windows real
DPAPI, las DACL y el Event Log son Windows-only: se validaron por interfaz/stub en Linux (build `GOOS=windows` ✓, `go test` ✓, govulncheck 0). Falta la pasada en un Windows real (mismo criterio que la firma del instalador): verificar que `file` sobre un `.pdf.enc` no diga "PDF document", que la DACL quede owner-only, y que los eventos aparezcan en el Visor de eventos.

## [1.1.0] — 2026-06-29

Autenticación por **token por instalación** con handshake de _pairing_. Cierra el bypass que tenía el control por CORS para requests sin header `Origin` (cualquier proceso local podía imprimir o leer la cola).

### Agregado
- **Token por instalación**: cada PC genera en el primer arranque un token de 256 bits (`crypto/rand`, base64url) que se persiste en `%LOCALAPPDATA%\PrinklyPrint\auth.json` y se reusa entre reinicios. Un equipo comprometido no expone a los demás.
- **Endpoint `POST /pair`**: handshake para que las apps web obtengan el token en runtime, sin que IT configure credenciales máquina por máquina. Si el origen ya está aprobado (`allowed_origins` / `allow_any_origin`) devuelve el token sin diálogo; si no, muestra un **diálogo nativo de confirmación** (por defecto en "Denegar") que muestra el origen de forma prominente.
- **Al aprobar un pareo, el origen se agrega a `allowed_origins`** — es decir, aparece en la lista visible y editable de **General → Orígenes CORS**. Es la única fuente de verdad de orígenes aprobados: para revocar un acceso, quitalo de esa lista (la próxima vez volverá a pedir aprobación). La lista se refresca sola en la UI cuando se aprueba un pareo nuevo.
- **Acción "pre-aprobar origen"**: agregar un dominio en `allowed_origins` (UI → General → Orígenes CORS) lo habilita a parearse sin diálogo, incluso en modo `--headless`.

### Cambiado
- **Los endpoints sensibles ahora exigen `Authorization: Bearer <token>`**: `GET /printers`, `GET /settings`, `POST /print`, `GET /jobs`, `GET /jobs/{id}`, `POST /jobs/{id}/retry`, `DELETE /jobs/{id}`. Token faltante o inválido → `401`, sin importar el `Origin`. Exentos: `GET /ping` (liveness) y `POST /pair`.
- **CORS pasó de estricto a permisivo**: el agente refleja cualquier `Origin` y responde el preflight. El control de acceso ya NO lo hace CORS sino el **token + el diálogo de pairing**. Era necesario: con CORS estricto el navegador bloqueaba el preflight de `POST /print` (origen no aprobado) ANTES de llegar al `401`, así que el pairing nunca podía arrancar desde un origen nuevo (la app ni siquiera podía detectar que el agente estaba corriendo). El preflight ahora habilita el header `Authorization`.
- **Requiere PrinklyPrint.js con soporte de pairing** (el cliente debe llamar a `/pair` y mandar el `Bearer` token). Integraciones por HTTP directo deben parearse y enviar el header. Ver README → Seguridad.

### Seguridad
- La puerta de acceso es el **token por instalación + el diálogo de aprobación** del operador (que muestra el origen, por defecto en "Denegar"). Un origen sin token recibe `401`; para obtener uno tiene que pasar por el diálogo. CORS dejó de ser una capa de control (era redundante con el token y rompía el pairing).
- **El origen del navegador se verifica en CADA request sensible**, no solo al parear: además del token, el `Origin` tiene que seguir en la lista de aprobados. Como el token es uno por instalación, esto evita que una app que ya lo tenga cacheado siga imprimiendo después de quitarla de la lista — **quitar un origen revoca su acceso al instante** (la app recibe `401` y vuelve a pedir aprobación). Los callers sin `Origin` (curl, Node, procesos locales) se gatean solo por el token.
- Cierra el bypass de "Origin ausente": procesos locales no-navegador ya no pueden usar la API sensible sin el token.
- El secreto vive en un archivo aparte del `config.yaml` editable por el usuario, con permisos restringidos best-effort (`0o600`; en Windows se apoya además en que `%LOCALAPPDATA%` es privado por usuario). _TODO: DACL explícita en Windows._
- El agente extiende el deadline de escritura de `/pair` mientras el diálogo está abierto, para no cortar la respuesta si el operador tarda en aprobar.

### Notas técnicas
- Nuevo paquete `internal/auth` (token de la instalación, threadsafe, comparación con `crypto/subtle.ConstantTimeCompare`). Los orígenes aprobados viven en `config.AllowedOrigins`, no en `auth.json`.
- Middleware de token por dentro del de CORS, así el preflight `OPTIONS` lo resuelve CORS antes del chequeo de token.
- Primeros tests unitarios del repo: `internal/auth` e `internal/server` (middleware, gate de `/pair`, CORS).

## [1.0.0] — 2026-05-23

Primer release público. La app está lista para producción en estaciones de trabajo Windows 10/11.

### Agregado
- **Agente de impresión silenciosa** para Windows que expone un servidor HTTP en `127.0.0.1:17777` para que aplicaciones web puedan imprimir PDFs sin diálogos del navegador.
- **UI nativa Win32** (lxn/walk) con 3 pestañas: Cola, Impresión y General. ~25 MB de RAM constante (vs ~85 MB de variantes basadas en WebView2).
- **Bandeja del sistema** con ícono semafórico (verde/amarillo/rojo) según estado de la cola y menú contextual traducido en vivo.
- **Multi-idioma**: Español, Inglés y Portugués. Autodetección desde el SO en el primer arranque.
- **SumatraPDF 3.5.2** embebido vía `go:embed` — no requiere instalar nada más.
- **Cola persistente** con SQLite (modernc.org/sqlite, sin CGO), reintentos con backoff exponencial y retención configurable.
- **CORS estricto** con whitelist de dominios y soporte de wildcards de subdominio.
- **Inicio automático con Windows** (toggle desde la UI; usa `HKCU\…\Run` — no requiere admin).
- **Instalador profesional** con Inno Setup: upgrades limpios sobre la versión anterior, acceso directo en menú inicio y opcionalmente en escritorio, cierra la instancia previa antes de actualizar el `.exe`.
- **Logging estructurado** JSON con rotación diaria en `%LOCALAPPDATA%\PrinklyPrint\logs\`.
- **Single-instance lock** vía mutex de kernel para que no convivan dos agentes en la misma PC.

### Notas técnicas
- Build 100% en Docker — no requiere instalar Go, Wine ni Inno Setup en la PC del desarrollador.
- Cross-compile desde Linux a `windows/amd64`, sin CGO.
- No usamos UPX: prioriza RAM constante sobre tamaño en disco.
