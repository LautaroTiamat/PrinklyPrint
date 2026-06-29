<div align="center">

<img src="icons/icon_256x256.png" alt="PrinklyPrint" width="160" height="160" />

# PrinklyPrint

**Imprimí PDFs desde tu web sin diálogos del navegador.**

Un agente local liviano para Windows que escucha en `127.0.0.1` (puerto `17777` por default, configurable) y manda los PDFs que le pase tu aplicación directo a la impresora — silencioso, persistente y sin dependencias externas.

[![Release](https://img.shields.io/github/v/release/LautaroTiamat/PrinklyPrint?style=flat-square&color=ec4899)](https://github.com/LautaroTiamat/PrinklyPrint/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-0078d4?style=flat-square)](#-instalación)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white)](go.mod)
[![Build with Docker](https://img.shields.io/badge/build-docker--only-2496ED?style=flat-square&logo=docker&logoColor=white)](#-build-desde-código-fuente)

### [⬇️  Descargar última versión (PrinklyPrint-Setup.exe)](https://github.com/LautaroTiamat/PrinklyPrint/releases/latest/download/PrinklyPrint-Setup.exe)

<sub>Ver todas las versiones publicadas → [GitHub Releases](https://github.com/LautaroTiamat/PrinklyPrint/releases)</sub>

</div>

---

## ¿Qué resuelve?

Los navegadores **no permiten imprimir silenciosamente**: siempre se muestra el diálogo "Imprimir". Para una web tipo POS, recepción de hotel, sucursal bancaria o despacho de paquetería —donde el operador imprime decenas o cientos de tickets por turno— ese diálogo es inviable.

**PrinklyPrint** es el agente local que vive en la PC del operador y resuelve eso: tu web le manda un PDF por HTTP y él lo imprime directo, sin diálogos, sin clicks extras.

```js
// En tu aplicación web:
await fetch('http://127.0.0.1:17777/print', {  // puerto configurable desde la UI
  method: 'POST',
  body: JSON.stringify({ pdf_base64: pdf, filename: 'ticket-123.pdf' })
});
// Listo: el ticket sale por la impresora del cliente.
```

---

## ✨ Características

| | |
|---|---|
| 🖨️  **Impresión silenciosa** | SumatraPDF embebido — no requiere instalar Acrobat ni otro PDF viewer. |
| ⚡  **Ultraliviano** | ~30 MB en disco, **~25 MB de RAM constante** (sin WebView ni Chromium embebido). |
| 🗂️  **Cola persistente** | SQLite local con reintentos automáticos. Si la impresora se queda sin papel, los jobs esperan. |
| 🔑  **Token + diálogo de aprobación** | Cada PC tiene su propio token. Los endpoints sensibles exigen `Authorization: Bearer`. Para emitir el token, la primera vez el agente te muestra un **diálogo nativo** preguntando si autorizás a ese sitio. Esa es la puerta real (no el string del Origin). |
| ✅  **Orígenes aprobados visibles** | Los sitios que aprobás quedan en la lista de **General → Orígenes CORS**, donde los ves y los podés quitar (quitar = revocar). Soporta wildcards de subdominio para pre-aprobar. |
| 🌍  **3 idiomas** | Español, Inglés y Portugués — autodetectados desde el SO. |
| 🚀  **Inicio con Windows** | Configurable desde la UI. Se activa por defecto al instalar. |
| 🔔  **Bandeja del sistema** | Ícono semafórico (verde/amarillo/rojo) que indica el estado de la cola a simple vista. |
| 🔄  **Upgrades sin fricción** | El instalador cierra la versión vieja y pisa los archivos limpiamente. |
| 📦  **Cero dependencias** | El `.exe` es un binario único. Nada de runtimes ni frameworks adicionales. |

---

## 📥 Instalación

### Para usuarios finales

1. Descargá **[PrinklyPrint-Setup.exe](https://github.com/LautaroTiamat/PrinklyPrint/releases/latest/download/PrinklyPrint-Setup.exe)** (siempre la última versión publicada).
2. Ejecutalo y seguí el asistente (se instala en `C:\Program Files\PrinklyPrint\`).
3. Listo: el ícono aparece en la bandeja del sistema. Por defecto se inicia con Windows y escucha en el puerto 17777 (lo podés cambiar desde la pestaña **General**).

> **¿Vas a actualizar?** Simplemente corré el nuevo instalador. PrinklyPrint detecta la versión instalada, cierra la instancia que está corriendo y reemplaza los archivos. Tu configuración y cola se preservan (viven en `%LOCALAPPDATA%\PrinklyPrint\`).

### Desinstalación

Inicio → Configuración → Aplicaciones → PrinklyPrint → Desinstalar. La carpeta de datos (`%LOCALAPPDATA%\PrinklyPrint\`) se conserva por si querés volver atrás; podés borrarla a mano si no la necesitás.

---

## 🚀 Uso desde tu aplicación web

### Opción 1: PrinklyPrint.js (recomendada)

La librería cliente oficial vive en su propio repositorio, con documentación completa, ejemplos para **React** y **vanilla JS**, y tipado TypeScript:

### 👉 **[github.com/LautaroTiamat/PrinklyPrint.js](https://github.com/LautaroTiamat/PrinklyPrint.js)**

Resumen de uso:

```js
import { PrinklyPrint } from 'prinklyprint.js';

// Por default apunta a http://127.0.0.1:17777.
// Pasale otro puerto si lo cambiaste desde la UI del agente.
const printer = new PrinklyPrint({ port: 17777 });

const blob = await fetch('/api/factura/123.pdf').then(r => r.blob());
await printer.print(blob, { filename: 'factura-123.pdf' });
```

### Opción 2: llamadas HTTP directas

El puerto por default es `17777` y se puede cambiar desde la pestaña **General** del agente — los ejemplos abajo asumen el default.

**Primero, obtené el token (pairing).** Los endpoints sensibles exigen `Authorization: Bearer <token>`. Para obtenerlo, hacé `POST /pair` desde el navegador: la primera vez el agente muestra un diálogo nativo para que el operador autorice tu dominio; una vez aprobado, devuelve el token (y las siguientes veces lo devuelve sin diálogo).

```js
// 1) Pairing: el navegador manda el Origin automáticamente.
const { token } = await fetch('http://127.0.0.1:17777/pair', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ label: 'Mi App de Facturación' }), // opcional, se muestra en el diálogo
}).then(r => r.json());

// 2) Usá el token en los endpoints sensibles.
await fetch('http://127.0.0.1:17777/print', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
  body: JSON.stringify({ pdf_base64: '...', filename: 'factura-123.pdf' }),
});
```

| Método | Endpoint | Token | Qué hace |
|--------|----------|:-----:|----------|
| `GET`  | `/ping` | — | Healthcheck. Devuelve `{ok, version, machine_id, paused}`. |
| `POST` | `/pair` | — | Handshake de pairing. Devuelve `{token}` (200) o `{error:"pairing_denied"}` (403). Body opcional: `{label}`. |
| `GET`  | `/printers` | ✔ | Lista impresoras del sistema con estado. |
| `GET`  | `/settings` | ✔ | Defaults de impresión configurados. |
| `POST` | `/print` | ✔ | Encola un PDF (body: `{pdf_base64, filename, options, metadata}`). |
| `GET`  | `/jobs` | ✔ | Lista jobs (filtros: `status`, `limit`, `offset`). |
| `GET`  | `/jobs/{id}` | ✔ | Detalle de un job específico. |
| `POST` | `/jobs/{id}/retry` | ✔ | Reencola un job fallido. |
| `DELETE` | `/jobs/{id}` | ✔ | Cancela un job en cola. |

> **Token faltante o inválido → `401`** (sin importar el `Origin`). **PrinklyPrint.js** hace todo el pairing por vos; usá la librería salvo que integres por HTTP a mano.
>
> **Pre-aprobar orígenes (sin diálogo / para `--headless`)**: agregá el dominio en **General → Orígenes CORS** (o en `allowed_origins` del `config.yaml`). Un origen pre-aprobado se parea sin diálogo, incluso en modo headless.

---

## ⚙️ Configuración

Al abrir la ventana de PrinklyPrint vas a encontrar 3 pestañas:

- **🖨️  Impresión** — impresora default, tamaño de papel, orientación, color, dúplex y escala.
- **📋  Cola** — jobs activos y completados con detalle de errores.
- **⚙️  General** — idioma, **puerto HTTP** (default `17777`, podés usar cualquier valor entre 1024–65535), orígenes CORS, inicio con Windows, retención y reintentos.

> **Cambiar el puerto**: si el `17777` está ocupado por otra app, o si tu organización requiere un puerto específico, abrí PrinklyPrint → **General → Puerto** y poné el que quieras. El cambio se aplica al reiniciar el agente.

La configuración se guarda en `%LOCALAPPDATA%\PrinklyPrint\config.yaml` y los cambios se aplican al instante.

```yaml
# Ejemplo de %LOCALAPPDATA%\PrinklyPrint\config.yaml
language: es
port: 17777                 # cambialo a lo que necesites (1024–65535)
allowed_origins:
  - https://miapp.empresa.com
  - https://*.empresa.com
auto_start: true
max_retries: 1
retention_days: 7
paper_size: A4
orientation: portrait
color: true
duplex: none
scale: fit
```

---

## 🏗️ Arquitectura

```
┌────────────────────────────────────────────────────────────┐
│ PC del operador                                            │
│                                                            │
│  Navegador (tu web)                                        │
│         │                                                  │
│         │  fetch('http://127.0.0.1:17777/print', {...})    │
│         ▼                                                  │
│  ┌───────────────────────────────────────────────┐         │
│  │ prinklyprint.exe                              │         │
│  │ ┌─────────────────────────────────────────┐   │         │
│  │ │ HTTP server (loopback + token + CORS)   │   │         │
│  │ │ Queue worker FIFO con reintentos        │   │         │
│  │ │ SQLite (jobs persistentes)              │   │         │
│  │ │ Tray icon (verde/amarillo/rojo)         │   │         │
│  │ │ Ventana Win32 (configuración + cola)    │   │         │
│  │ └─────────────────────────────────────────┘   │         │
│  └─────────────────────┬─────────────────────────┘         │
│                        │                                   │
│                        ▼                                   │
│              SumatraPDF.exe (embebido)                     │
│                        │                                   │
│                        ▼                                   │
│            Impresora local (spooler Win32)                 │
└────────────────────────────────────────────────────────────┘
```

| Package | Responsabilidad |
|---------|-----------------|
| [`app`](internal/app/) | Bootstrap, ciclo de vida, single-instance lock, dialogs Win32. |
| [`autostart`](internal/autostart/) | Toggle "Iniciar con Windows" vía `HKCU\…\Run`. |
| [`config`](internal/config/) | Persistencia YAML threadsafe en `%LOCALAPPDATA%\PrinklyPrint\config.yaml`. |
| [`auth`](internal/auth/) | Token por instalación + orígenes pareados (`auth.json`, threadsafe). |
| [`store`](internal/store/) | SQLite (modernc.org/sqlite, sin CGO). |
| [`printer`](internal/printer/) | `EnumPrinters` + wrapper SumatraPDF + pre-flight check. |
| [`queue`](internal/queue/) | Worker FIFO con backoff exponencial y limpieza periódica. |
| [`server`](internal/server/) | HTTP API con token Bearer + CORS estricto. |
| [`tray`](internal/tray/) | Ícono de bandeja con i18n reactivo y menú contextual. |
| [`ui`](internal/ui/) | Ventana nativa Win32 con `lxn/walk` (3 pestañas). |
| [`i18n`](internal/i18n/) | Diccionario ES/EN/PT. |
| [`locale`](internal/locale/) | Autodetección del idioma del SO. |
| [`logging`](internal/logging/) | `log/slog` JSON con rotación diaria. |
| [`embedded`](internal/embedded/) | SumatraPDF.exe vía `go:embed`. |

---

## 🛠️ Build desde código fuente

**Requisito único**: tener [Docker](https://www.docker.com/products/docker-desktop/) instalado.

### Compilar el `.exe` localmente

```bash
docker build -t prinklyprint-build .
docker run --rm -v "$PWD:/work" -e VERSION=1.0.0 prinklyprint-build
# → dist/prinklyprint.exe
```

### Versionar un release público

El instalador (`PrinklyPrint-Setup.exe`, sin versión en el nombre para que el link `releases/latest/download/PrinklyPrint-Setup.exe` siempre funcione; la versión va embebida en `VersionInfoVersion`) se compila automáticamente en GitHub Actions sobre un runner Windows nativo (donde Inno Setup funciona sin Wine). Para publicar:

```bash
git tag v1.1.0
git push origin v1.1.0
```

Eso dispara [`.github/workflows/release.yml`](.github/workflows/release.yml), que:

1. Compila `prinklyprint.exe` en Ubuntu (cross-compile a `windows/amd64`).
2. Compila el instalador en Windows con Inno Setup nativo.
3. Crea un **GitHub Release** con el `.exe` y el `Setup.exe` adjuntos y las release notes tomadas de [`CHANGELOG.md`](CHANGELOG.md).

El número de versión queda embebido en el `.exe` (visible con `prinklyprint.exe --version` y en Panel de control → Programas → "Acerca de").

> **Nota sobre Inno Setup local**: la compilación del instalador NO funciona bajo Docker Desktop / WSL2 porque Wine + el kernel WSL2 fallan en `socket(SOCK_SEQPACKET)`. Si querés generarlo en tu máquina sin esperar a CI, la única opción es instalar Inno Setup nativo en Windows (`winget install JRSoftware.InnoSetup`) y correr `iscc /DAppVersion=1.0.0 installer/setup.iss` desde la raíz del repo.

---

## 🔐 Seguridad de la cadena de suministro (supply chain)

El pipeline está endurecido para resistir ataques de supply chain y producir evidencia auditable.

### Qué está pineado / verificado

- **GitHub Actions por commit SHA** (no por tag mutable como `@v4`). Ver los `uses: …@<sha> # vX.Y.Z` en [`release.yml`](.github/workflows/release.yml) y [`security.yml`](.github/workflows/security.yml).
- **Imagen base del Dockerfile por digest** (`golang:1.25-alpine@sha256:…`), no por tag.
- **rsrc** fijado en `go.mod`/`go.sum` vía [`tools.go`](tools.go) y ejecutado con `go run` (integridad de `go.sum`); en el Docker, `go install …@v0.10.2`.
- **SumatraPDF verificado por SHA256** contra [`checksums.txt`](checksums.txt): el build **FALLA** si el binario descargado no coincide.
- **Build reproducible**: `-trimpath`, `-ldflags "-buildid="`, `CGO_ENABLED=0` explícito.
- **[Dependabot](.github/dependabot.yml)** mantiene al día (vía PRs) las actions, los módulos Go y la imagen Docker.

### Escaneos de seguridad ([`security.yml`](.github/workflows/security.yml))

Corre en push, PR a `main` y semanalmente:

| Chequeo | Herramienta | Qué cubre |
|---|---|---|
| SCA | **govulncheck** | Vulnerabilidades en dependencias y stdlib que el código alcanza. |
| SAST | **gosec** | Patrones inseguros en el código Go (sube SARIF al tab Security). |
| SAST | **CodeQL** | Análisis profundo de Go (`build-mode: none`). |
| Secretos | **gitleaks** | Credenciales filtradas en el historial. |

> **Repo privado**: CodeQL y la subida de SARIF requieren GitHub Advanced Security. Este repo es público (gratis). Si lo hacés privado sin GHAS, comentá el job `codeql` y el `upload-sarif` de gosec y agregá `semgrep` (no requiere GHAS).

### Firma de binarios (Authenticode)

El `.exe` **y** el instalador se firman en el runner Windows ([`.github/scripts/sign.ps1`](.github/scripts/sign.ps1)) con timestamp RFC3161. Cargá en **Settings → Secrets and variables → Actions**:

- **Opción A (.pfx)**: secret `WINDOWS_CERT_PFX_BASE64` (el `.pfx` en base64) + secret `WINDOWS_CERT_PASSWORD`.
  ```powershell
  [Convert]::ToBase64String([IO.File]::ReadAllBytes('cert.pfx')) | Set-Clipboard
  ```
- **Opción B (servicio externo / HSM)**: secret `SIGN_COMMAND` (el `{FILE}` se reemplaza por la ruta a firmar). Sirve para Azure Trusted Signing, DigiCert smctl, etc.
- **Opcional**: variable `WINDOWS_TIMESTAMP_URL` (default `http://timestamp.digicert.com`).

Si no hay credenciales, el build **no falla** pero los binarios salen **sin firmar** (warn + skip). El release de producción debe tenerlas cargadas.

### SBOM y procedencia (provenance)

Cada release adjunta:
- **`bom.json`** — SBOM CycloneDX (generado con `cyclonedx-gomod`).
- **Attestation de build provenance** ([`actions/attest-build-provenance`](https://github.com/actions/attest-build-provenance)) del `.exe` y el instalador.

Verificación (necesita [`gh`](https://cli.github.com/)):

```bash
# Procedencia del binario descargado del release:
gh attestation verify prinklyprint.exe --repo LautaroTiamat/PrinklyPrint
gh attestation verify PrinklyPrint-Setup.exe --repo LautaroTiamat/PrinklyPrint

# SBOM: es JSON CycloneDX estándar; inspeccionalo con jq o cualquier tooling SCA.
jq '.components[].name' bom.json
```

### Settings del repo que hay que activar (no alcanza con los workflows)

Para que los chequeos **bloqueen** merges y se cierre el control de gestión de vulnerabilidades:

1. **Settings → Branches → Branch protection rule** en `main`: *Require status checks to pass* → marcá `govulncheck`, `gosec`, `CodeQL`, `gitleaks`.
2. **Settings → Code security → Code scanning**: marcá los resultados como *required*.
3. **Settings → Code security → Secret scanning**: activá *Secret scanning* y *Push protection*.

---

## 🔒 Datos en reposo, validación y auditoría

### Cifrado de PDFs en reposo (DPAPI)

Los PDFs son el dato más sensible. En disco se guardan **cifrados** como `<id>.pdf.enc` usando la **DPAPI de Windows** (`CryptProtectData`) con **scope de usuario**: la clave deriva del perfil del usuario que corre el agente, así que un archivo copiado a otro usuario u otra PC **no se puede descifrar**. Un `file` sobre un `.pdf.enc` no dice "PDF document".

Para imprimir, el agente descifra el PDF a un **archivo temporal** (en `%LOCALAPPDATA%\PrinklyPrint\print-tmp\`) con permisos restringidos, se lo pasa a SumatraPDF y lo **borra inmediatamente** después (sobreescritura best-effort + remove). Existe una ventana de "plano" inevitable mientras SumatraPDF lee el archivo, mitigada por la ACL owner-only + el borrado inmediato.

> En no-Windows (solo dev/CI) el cifrado es un passthrough sin cifrar — el agente productivo corre solo en Windows.

### Permisos owner-only (ACL)

`auth.json`, `agent.db` (+ sidecars `-wal`/`-shm`), `config.yaml`, y los directorios de logs, PDFs y temporales quedan con una **DACL protegida** (sin herencia) que concede acceso solo al **dueño + SYSTEM + Administradores locales**. En Unix (dev/CI) es `0o600`/`0o700`.

### Validación de entrada en `/print`

Antes de encolar, el agente valida: que el contenido sea realmente un PDF (magic bytes `%PDF-`), `copies` ≤ 999, `orientation`/`duplex`/`scale` como enums cerrados, `paper_size` con charset seguro, y **`page_range` con whitelist estricta** (`^[0-9,\- ]*$`, para no inyectar directivas a SumatraPDF). `metadata` se topa en 16 KB, `GET /jobs` acota `limit` a 1..500, y la cola rechaza con `429 queue_full` si hay 1000+ jobs pendientes.

### Sin conexiones salientes (SSRF eliminado)

`pdf_url` (descarga remota del PDF) **fue eliminado por completo**. El agente solo imprime PDFs enviados **inline** en `pdf_base64`; los genera el backend/frontend del cliente y se mandan en el body de `/print`. El agente **no realiza ninguna conexión saliente de red**, lo que elimina de raíz la clase de vulnerabilidad **SSRF** (en vez de blindarla). Un body con `pdf_url` se ignora como campo desconocido y, al faltar `pdf_base64`, recibe `400 bad_request`.

### Eventos de seguridad y SIEM

El agente emite eventos de seguridad al **Windows Event Log** (canal *Application*, source `PrinklyPrint`), además del log de archivo. Un **SIEM corporativo** los recolecta de forma centralizada (es ahí donde vive la integridad/retención real; el archivo local es complementario). Eventos e IDs:

| ID | Evento | Cuándo |
|----|--------|--------|
| 1001 | `auth_failure` | request a un endpoint sensible rechazado (401) — token faltante/inválido u origen no aprobado |
| 1002 | `pairing_approved` | se autorizó un origen |
| 1003 | `pairing_denied` | pareo rechazado |
| 1004 | `print_enqueued` | se encoló un job |
| 1005 | `settings_changed` | cambió la configuración |
| 1006 | `token_rotated` | se rotó el token de la instalación |

> Nunca se loguea el valor del token.

**Registro del source** (requiere admin): lo hace el **instalador** automáticamente (`prinklyprint --register-eventlog`, elevado). Si instalaste a mano o el agente avisa que el Event Log no está disponible, corré una vez en una consola **como administrador**:

```powershell
& "C:\Program Files\PrinklyPrint\prinklyprint.exe" --register-eventlog
```

Si no está registrado, el agente sigue funcionando y loguea **solo a archivo** (con un warning).

### Documentación de seguridad y operación

Documentación detallada para equipos de seguridad y operaciones, en [`docs/`](docs/):

| Documento | Para qué |
|-----------|----------|
| [`SECURITY.md`](SECURITY.md) | Política de divulgación responsable, versiones soportadas y resumen de la postura de seguridad. |
| [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md) | Modelo de amenazas: activos, límites de confianza, amenazas (STRIDE) y riesgos residuales. |
| [`docs/DEPLOYMENT_HARDENING.md`](docs/DEPLOYMENT_HARDENING.md) | Despliegue y hardening on-premise (identidad de ejecución, GPO, permisos, postura de red). |
| [`docs/RUNBOOK.md`](docs/RUNBOOK.md) | Runbook operativo: salud, logs, rotación de token, troubleshooting e incidentes. |
| [`docs/PENTEST_GUIDE.md`](docs/PENTEST_GUIDE.md) | Guía de pruebas de seguridad con PoCs y resultados esperados, para verificación externa. |

---

## 📁 Estructura del proyecto

```
PrinklyPrint/
├── main.go                       Entry point
├── app.manifest                  Activa Common Controls v6 + DPI awareness
├── go.mod                        module github.com/lautarotiamat/prinklyprint
├── LICENSE                       MIT
├── CHANGELOG.md
├── icons/                        Logo en SVG + .ico multi-frame + PNGs
├── installer/
│   └── setup.iss                 Inno Setup → PrinklyPrint-Setup.exe
└── internal/
    ├── app/                      Bootstrap + singleton + dialogs Win32
    ├── autostart/                HKCU\…\Run toggle
    ├── config/                   YAML threadsafe
    ├── auth/                     Token por instalación + orígenes pareados
    ├── winfs/                    Permisos owner-only (DACL Windows / chmod stub)
    ├── crypto/dpapi/             Cifrado en reposo (DPAPI) de los PDFs
    ├── seclog/                   Eventos de seguridad → slog + Event Log (SIEM)
    ├── store/                    SQLite (sin CGO)
    ├── printer/                  EnumPrinters + SumatraPDF
    ├── queue/                    Worker FIFO con reintentos
    ├── server/                   HTTP + token Bearer + CORS estricto
    ├── tray/                     Ícono de bandeja con i18n reactivo
    ├── ui/                       Ventana Win32 (lxn/walk)
    ├── i18n/                     Diccionario ES/EN/PT
    ├── locale/                   Autodetect del SO
    ├── logging/                  slog JSON con rotación diaria
    └── embedded/                 SumatraPDF embebido (go:embed)
```

---

## 🐛 Logs y troubleshooting

Los logs viven en `%LOCALAPPDATA%\PrinklyPrint\logs\agent-YYYY-MM-DD.log` (rotación diaria, retención 14 días).

```powershell
# Seguir el log en vivo
Get-Content -Wait -Tail 50 "$env:LOCALAPPDATA\PrinklyPrint\logs\agent-$(Get-Date -Format yyyy-MM-dd).log"
```

Cada job se loguea con `job_id`, `filename`, `printer`, `attempt`, `metadata` y resultado — útil para auditar quién imprimió qué y cuándo.

---

## ❓ FAQ

**¿Funciona en Linux o macOS?**
No. PrinklyPrint usa APIs de Win32 directamente (`lxn/walk`, `EnumPrinters`, manifest XML, `HKCU\Run`). Portar requeriría reescribir las capas de UI, tray y autostart. Hoy es Windows-only por diseño.

**¿Por qué SumatraPDF y no `gs`/Foxit/Acrobat?**
SumatraPDF es liviano (~9 MB), permisivo en su licencia, y soporta impresión silenciosa por línea de comandos sin requerir instalación previa. Lo embebemos con `go:embed` en el `.exe`.

**¿Es seguro abrir un puerto local?**
Sí, con dos capas. El server bindea **solo a `127.0.0.1`** (loopback) — no es accesible desde la red. Sobre eso:

- **Token por instalación**: los endpoints sensibles (`/print`, `/jobs`, `/printers`, `/settings`, …) exigen `Authorization: Bearer <token>`. Sin token válido → `401`, sin importar el `Origin`. Esto bloquea a cualquier proceso local no-navegador que antes podía llamar al loopback. El token se genera por equipo en el primer arranque y vive en `%LOCALAPPDATA%\PrinklyPrint\auth.json` (aparte del `config.yaml`).
- **Pairing**: las apps obtienen el token con `POST /pair`. La primera vez para un origen nuevo, el operador ve un **diálogo nativo** (por defecto en "Denegar") que muestra el origen; recién al aprobarlo se entrega el token. Así una página web maliciosa no puede parearse sola sin que alguien confirme. **Al aprobar, el origen queda en la lista de General → Orígenes CORS**, donde lo ves y lo podés quitar — quitarlo revoca el acceso y vuelve a pedir aprobación la próxima vez.
- **CORS permisivo a propósito**: el agente acepta requests de cualquier origen a nivel CORS. La protección NO es CORS sino el token + el diálogo. Si CORS fuera estricto, el navegador bloquearía el request de un origen nuevo *antes* del `401`, y el pairing nunca podría arrancar. Un origen no aprobado igual no puede imprimir: sin token recibe `401`, y para conseguir token tiene que pasar por el diálogo.

Exentos de token: `GET /ping` (liveness) y `POST /pair` (es quien emite el token). Para entornos `--headless` (sin UI para el diálogo), pre-aprobá los orígenes en `allowed_origins`.

**¿Cómo revoco el acceso de una app, o fuerzo el re-pairing?**
Para revocar **una app puntual**: quitá su origen de **General → Orígenes CORS**. Deja de estar autorizada y la próxima vez que intente, el agente volverá a pedir aprobación. Para invalidar **todos** los tokens de golpe (sospecha de filtración): borrá `%LOCALAPPDATA%\PrinklyPrint\auth.json` y reiniciá el agente — genera un token nuevo y las apps tendrán que re-parear (silencioso para los orígenes que sigan aprobados en la lista). _(Una acción "Regenerar token" en la UI está planificada.)_

**¿Y si necesito que arranque antes de que un usuario haga login?**
PrinklyPrint admite el flag `--headless` (sin UI ni bandeja, solo server + cola). Podés registrar una scheduled task con cuenta SYSTEM si tu caso requiere arranque pre-login.

**¿Conserva los jobs si reinicio la PC?**
Sí. La cola está persistida en SQLite (`%LOCALAPPDATA%\PrinklyPrint\agent.db`). Al arrancar se reanuda automáticamente.

---

## 📜 Licencia

[MIT](LICENSE) © 2026 [LautaroTiamat](https://github.com/LautaroTiamat).
