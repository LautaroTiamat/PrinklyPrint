# Changelog

Todas las versiones notables de PrinklyPrint quedan documentadas acá. Formato basado en [Keep a Changelog](https://keepachangelog.com/es/1.1.0/) y el proyecto sigue [Semantic Versioning](https://semver.org/lang/es/).

## [1.4.0] — 2026-08-24

### Agregado
- **Botón «Regenerar token» en General**: rota el token de la instalación en
  caliente (confirmación con «No» por default) y emite el evento SIEM 1006
  (`token_rotated`), que hasta ahora estaba definido pero nunca se emitía.
  El procedimiento de emergencia (borrar `auth.json`) sigue disponible.
- **Contrato de API** (`contract/api-shapes.json` + `TestAPIContract`): fija la
  forma (claves + tipos JSON) de cada respuesta del agente, generada desde los
  handlers reales. La librería PrinklyPrint.js lleva una copia y un test espejo
  que valida sus tipos TypeScript contra ella — el drift Go↔TS (como el
  traslado de `machine_id` de `/ping` a `/settings`) ahora rompe un test.
  Regenerar con `CONTRACT_UPDATE=1 go test ./internal/server -run TestAPIContract`.
- **Flag `--data-dir`**: permite aislar una instancia de prueba (tests, pentest)
  del data dir real en `%LOCALAPPDATA%`.

### Cambiado
- **Bandeja del sistema migrada de fyne.io/systray a walk.NotifyIcon** (una
  dependencia menos y el comportamiento estándar de Windows): el menú se abre
  SOLO con click derecho, el doble click izquierdo abre la ventana principal, y
  el click izquierdo simple no hace nada. Antes cualquier click abría el menú.
- **Abrir desde la bandeja trae la ventana al frente** como cualquier app
  (restaura si estaba minimizada, sin des-maximizar, y pide el foreground).
- **La pausa de la cola persiste y es consistente.** Antes, `paused: true` en el
  `config.yaml` se ignoraba al arrancar (la pausa no sobrevivía reinicios) y el
  menú del tray pausaba el worker sin persistir la config, con lo cual
  `GET /ping` (lee el worker) y `GET /settings` (lee la config) podían reportar
  `paused` distinto. Ahora el worker arranca pausado si la config lo indica y el
  tray persiste cada cambio.
- **`max_retries` ahora significa lo que dice.** Con el default `1`, el primer
  fallo marcaba el job como `failed` sin ningún reintento (la comparación era
  contra intentos totales). Ahora `max_retries` cuenta **reintentos después del
  intento inicial**: con `1`, un job se reintenta una vez (backoff 5s) antes de
  quedar `failed`.
- **`max_retries` y `retention_days` aplican en vivo.** El worker los congelaba
  al construirse: cambiarlos desde la pestaña General recién surtía efecto al
  reiniciar el agente. Ahora el worker consulta la config vigente en cada uso.
- **Rango de puerto unificado a 1024–65535.** La validación de config aceptaba
  1–65535 mientras la UI y el README decían 1024–65535; un `config.yaml` editado
  a mano con un puerto privilegiado (80) pasaba la validación y fallaba al
  bindear. Las configs existentes no se ven afectadas (solo se valida al
  guardar cambios).
- **La versión del assembly manifest ya no queda hardcodeada en `0.1.0.0`**: el
  build (Dockerfile y release.yml) sustituye el placeholder `0.0.0.0` de
  `app.manifest` por la versión real antes de generar `rsrc.syso`.
- **Un origen con barra final ya matchea.** El operador que escribía
  `https://app.empresa.com/` en Orígenes CORS (o con espacios, o con otra
  capitalización) nunca matcheaba el header `Origin` del navegador y el agente
  no reconocía al cliente. Ahora el match normaliza ambos lados (espacios y
  `/` finales fuera, comparación case-insensitive) — también para wildcards
  (`*.empresa.com/`) —, la UI guarda el valor ya normalizado y la aprobación
  de pairing también. Las entradas sin esquema siguen matcheando http y https
  por diseño (intranets sin TLS).
- **La pestaña Impresión ya no pisa la config guardada.** El refresco periódico
  de la lista de impresoras disparaba los mismos eventos que un click humano
  con los combos a medio armar, y persistía ese estado intermedio: la impresora
  default elegida volvía a «(default del sistema)» sola — por eso la
  configuración parecía no guardarse entre reinicios. El refresh programático
  ahora no persiste, y el modelo del combo solo se resetea si la lista de
  impresoras realmente cambió (tampoco se cierra más el dropdown solo).
- **Los refresh automáticos de la ventana corren en el hilo de UI.** Se
  llamaban directo desde goroutines (prohibido en Win32) y corrompían el estado
  de la ventana — la causa más probable de que una ventana maximizada dejara de
  responder a cerrar/minimizar/restaurar. Ahora todo pasa por `Synchronize`.

### Documentación
- `docs/THREAT_MODEL.md`: la sección «validación en Windows real» refleja lo
  verificado sobre una instalación de producción 1.3.1 (Windows 11 IoT LTSC):
  cifrado DPAPI en reposo (blob con GUID `d08c9ddf…`, no un PDF legible), DACL
  owner-only exacta vía `icacls`, y eventos 1001/1004/1005 emitidos y
  consultables en el Event Log. Queda pendiente solo la firma con certificado real.
- `SECURITY.md`: tabla de versiones soportadas actualizada a 1.3.x.
- `docs/THREAT_MODEL.md` y `docs/DEPLOYMENT_HARDENING.md`: la DACL owner-only
  figura como **validada en Windows real** (lo estaba desde 1.2.1) y el SDDL
  documentado ahora es el vigente (SID explícito + ACEs heredables en
  directorios), no el de 1.2.0.
- README: se agregaron `winfs`, `crypto/dpapi`, `seclog` e `insecure` a la tabla
  de paquetes (e `insecure` al árbol), y el ejemplo de config documenta la
  semántica real de `max_retries` y la persistencia de `paused`.

### Seguridad
- **Hardening de permisos de directorio**: los directorios privados que crea el
  agente (auth, config, `bin` de SumatraPDF) pasan de `0o755` a `0o700`. En
  Windows —único SO de producción— la protección real la da la DACL de `winfs`;
  en Unix/CI es lo correcto. Sin cambios de comportamiento.
- **Análisis estático (gosec) sin ruido**: se anotaron con `// #nosec <regla> --
  <motivo>` los hallazgos verificados como falsos positivos o comportamiento por
  diseño (subproceso de SumatraPDF con argumentos validados; lecturas/escrituras
  de archivos con rutas internas, no input externo; bit de ejecución del binario
  de SumatraPDF; permisos de directorio del stub no-Windows). La justificación
  queda **en el código** para que sea auditable, no oculta en la UI del scanner.
  No se modificó ninguna lógica de seguridad.

## [1.3.0] — 2026-06-29

Tres cambios pedidos por una revisión de seguridad. No tocan el flujo de
impresión (`/print`, cola, ritmo) ni la lógica de auth/pairing/token.

### Agregado
- **Rate limit opcional de `POST /pair`** (apagado por default). Token bucket
  (tasa sostenida + ráfaga) **global** para `/pair`; al superarse responde
  `429 rate_limited` con header `Retry-After`. Configurable desde la UI (pestaña
  General) o el `config.yaml` (`pair_rate_limit_enabled` / `pair_rate_limit_per_minute`
  / `pair_rate_limit_burst`), con lectura en vivo. **No afecta la impresión**:
  `/print` tiene sus propios límites (tamaño de body y profundidad de cola) y
  nunca pasa por este limitador. Un rechazo se registra en el SIEM
  (`pairing_denied`, reason `rate_limited`).

### Cambiado
- **`allow_any_origin` solo se habilita desde el instalador.** El modo "permitir
  cualquier origen" (CORS permisivo que entrega el token sin diálogo) dejó de
  poder activarse en runtime: ya no vive en `config.yaml` ni en la UI. La única
  fuente de verdad es una marca en el registro (`HKLM\Software\PrinklyPrint\AllowAnyOrigin`),
  escribible solo por un proceso elevado (el instalador). La UI muestra el estado
  como **solo lectura** (con advertencia si está ACTIVADO). Si el agente arranca
  con el modo activo, emite un evento de seguridad WARNING (`insecure_mode_enabled`)
  al Event Log/SIEM.
- **`GET /ping` ya no expone `machine_id`.** El healthcheck sin token ahora
  devuelve solo `{ok, version, paused}`. `machine_id` se obtiene desde
  `GET /settings`, que exige token.

### Seguridad
- Reduce la superficie del endpoint sin autenticación (`/ping`) y elimina la
  posibilidad de activar por error un modo CORS inseguro en runtime. El rate
  limit de `/pair` agrega protección contra fuerza bruta del pareo sin tocar el
  flujo de impresión.

### Instalador
- Nueva opción **"Permitir cualquier origen CORS (NO recomendado)"** (desmarcada
  por default) que escribe la marca en HKLM. Soporta instalación silenciosa/GPO
  con `/ALLOWANYORIGIN=1`. El desinstalador limpia la marca.

## [1.2.1] — 2026-06-29

### Corregido
- **El agente no arrancaba** (la ventana/ícono de bandeja no aparecían y el proceso salía en silencio, al ser GUI sin consola). Causa: los permisos owner-only (`internal/winfs`) aplicaban las ACEs **sin marca de herencia**, así que los archivos creados dentro de los directorios restringidos (logs rotados, PDFs, DB) quedaban con una **DACL vacía** y el propio agente no podía reabrirlos en el siguiente arranque (`abrir log: Access is denied`). Además se concedía acceso vía `OWNER RIGHTS` (S-1-3-4), de semántica ambigua.
  - Ahora `winfs.Restrict` concede el **SID explícito del usuario** que corre el agente (es el mismo en un token normal o elevado) en lugar de `OWNER RIGHTS`, y en **directorios** aplica ACEs **heredables** (`OICI`) para que los archivos hereden el acceso. Verificado en Windows real: el agente arranca, extrae SumatraPDF, crea logs accesibles y responde `GET /ping`.
- **Al actualizar desde 1.2.0**: si el agente no abría, después de instalar 1.2.1 conviene borrar `%LOCALAPPDATA%\PrinklyPrint` una vez (se regenera solo) para limpiar los archivos que quedaron con permisos rotos. Esto regenera el token de la instalación (las apps vuelven a parear).

## [1.2.0] — 2026-06-29

Endurecimiento de seguridad en dos frentes: la **cadena de suministro** del pipeline de build/release (con evidencia auditable) y el **agente** en sí, cerrando tres brechas de seguridad — datos en reposo sin cifrar, validación de entrada incompleta y logging no integrado a SIEM.

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
- Cierra varias brechas de seguridad: integridad de pipeline, integridad de artefactos / OWASP A08 (firma + provenance + verificación de hash), SBOM, pruebas de seguridad, gestión de vulnerabilidades (govulncheck + Dependabot + bump de Go), **datos en reposo (cifrado DPAPI + ACL owner-only), validación de entrada y logging/SIEM**.
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
