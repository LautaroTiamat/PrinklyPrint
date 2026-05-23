<div align="center">

<img src="icons/icon_256x256.png" alt="PrinklyPrint" width="160" height="160" />

# PrinklyPrint

**Imprimí PDFs desde tu web sin diálogos del navegador.**

Un agente local liviano para Windows que escucha en `127.0.0.1` (puerto `17777` por default, configurable) y manda los PDFs que le pase tu aplicación directo a la impresora — silencioso, persistente y sin dependencias externas.

[![Release](https://img.shields.io/github/v/release/LautaroTiamat/PrinklyPrint?style=flat-square&color=ec4899)](https://github.com/LautaroTiamat/PrinklyPrint/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-0078d4?style=flat-square)](#-instalación)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?style=flat-square&logo=go&logoColor=white)](go.mod)
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
| 🔒  **CORS estricto** | Solo aceptan llamadas los dominios que vos autorices. Soporta wildcards de subdominio. |
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

| Método | Endpoint | Qué hace |
|--------|----------|----------|
| `GET`  | `/ping` | Healthcheck. Devuelve `{ok, version, machine_id, paused}`. |
| `GET`  | `/printers` | Lista impresoras del sistema con estado. |
| `GET`  | `/settings` | Defaults de impresión configurados. |
| `POST` | `/print` | Encola un PDF (body: `{pdf_base64, filename, options, metadata}`). |
| `GET`  | `/jobs` | Lista jobs (filtros: `status`, `limit`, `offset`). |
| `GET`  | `/jobs/{id}` | Detalle de un job específico. |
| `POST` | `/jobs/{id}/retry` | Reencola un job fallido. |
| `DELETE` | `/jobs/{id}` | Cancela un job en cola. |

> Antes de que tu web pueda imprimir, agregá su dominio en la pestaña **General → Orígenes CORS** del agente.

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
│  │ │ HTTP server (loopback + CORS estricto)  │   │         │
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
| [`store`](internal/store/) | SQLite (modernc.org/sqlite, sin CGO). |
| [`printer`](internal/printer/) | `EnumPrinters` + wrapper SumatraPDF + pre-flight check. |
| [`queue`](internal/queue/) | Worker FIFO con backoff exponencial y limpieza periódica. |
| [`server`](internal/server/) | HTTP API con CORS estricto. |
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

El instalador (`PrinklyPrint-Setup-X.Y.Z.exe`) se compila automáticamente en GitHub Actions sobre un runner Windows nativo (donde Inno Setup funciona sin Wine). Para publicar:

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
│   └── setup.iss                 Inno Setup → PrinklyPrint-Setup-{ver}.exe
└── internal/
    ├── app/                      Bootstrap + singleton + dialogs Win32
    ├── autostart/                HKCU\…\Run toggle
    ├── config/                   YAML threadsafe
    ├── store/                    SQLite (sin CGO)
    ├── printer/                  EnumPrinters + SumatraPDF
    ├── queue/                    Worker FIFO con reintentos
    ├── server/                   HTTP + CORS estricto
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
El server bindea **solo a `127.0.0.1`** (loopback) — no es accesible desde la red. Además aplica CORS estricto: solo los dominios que vos autorizás pueden invocarlo desde el navegador.

**¿Y si necesito que arranque antes de que un usuario haga login?**
PrinklyPrint admite el flag `--headless` (sin UI ni bandeja, solo server + cola). Podés registrar una scheduled task con cuenta SYSTEM si tu caso requiere arranque pre-login.

**¿Conserva los jobs si reinicio la PC?**
Sí. La cola está persistida en SQLite (`%LOCALAPPDATA%\PrinklyPrint\agent.db`). Al arrancar se reanuda automáticamente.

---

## 📜 Licencia

[MIT](LICENSE) © 2026 [LautaroTiamat](https://github.com/LautaroTiamat).
