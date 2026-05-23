# Changelog

Todas las versiones notables de PrinklyPrint quedan documentadas acá. Formato basado en [Keep a Changelog](https://keepachangelog.com/es/1.1.0/) y el proyecto sigue [Semantic Versioning](https://semver.org/lang/es/).

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
