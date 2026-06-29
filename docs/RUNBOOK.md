# Runbook operativo — PrinklyPrint

Procedimientos para operar, diagnosticar y responder incidentes del agente.
Neutro y basado en el comportamiento real. Los SLAs y los contactos de
escalamiento los define cada organización.

En los ejemplos, `$PORT` es el puerto configurado (default `17777`) y `$TOKEN` es
el token de instalación (ver más abajo cómo obtenerlo).

---

## 1. Verificar salud

`GET /ping` está **exento de token** y sirve de liveness. Expone lo mínimo:
`ok`, `version`, `paused`. (El `machine_id` ya **no** va en `/ping`; se obtiene
desde `GET /settings`, que exige token.)

```powershell
Invoke-RestMethod -Uri "http://127.0.0.1:17777/ping"
# -> ok=True; version=...; paused=False
```

- Si responde: el agente está corriendo.
- Si da error de conexión: el agente no está arrancado (revisá el ícono de
  bandeja, la entrada de autostart `HKCU\...\Run`, o arrancalo a mano).
- `paused=True` indica que el worker de cola está pausado (no procesa jobs aunque
  los acepte). Se reanuda desde la UI.

> Nota sobre proxys: herramientas como Burp/ZAP suelen **bypassear** `localhost`.
> Para inspeccionar el tráfico, configurá el proxy explícito a `127.0.0.1` o usá
> `curl`/`Invoke-WebRequest` directo.

---

## 2. Logs

Dos destinos:

- **Archivo** (complementario, local): `%LOCALAPPDATA%\PrinklyPrint\logs\agent-YYYY-MM-DD.log`
  (rotación diaria).
- **Windows Event Log** (para el SIEM): canal **Application**, source
  **PrinklyPrint**, eventos de seguridad con IDs estables.

```powershell
# Seguir el log de archivo en vivo
Get-Content -Wait -Tail 50 "$env:LOCALAPPDATA\PrinklyPrint\logs\agent-$(Get-Date -Format yyyy-MM-dd).log"

# Ver los eventos de seguridad en el Event Log
Get-WinEvent -ProviderName PrinklyPrint -MaxEvents 50 | Format-Table -Auto
```

Eventos de seguridad (IDs):

| ID | Evento | Cuándo |
|----|--------|--------|
| 1001 | `auth_failure` | request a endpoint sensible rechazado (401): token faltante/inválido u origen no aprobado |
| 1002 | `pairing_approved` | se autorizó un origen |
| 1003 | `pairing_denied` | pareo rechazado |
| 1004 | `print_enqueued` | se encoló un job |
| 1005 | `settings_changed` | cambió la configuración |
| 1006 | `token_rotated` | se rotó el token de la instalación |

> El **token nunca** aparece en los logs ni en los eventos. Los campos
> controlados por el cliente (filename, Origin) se sanean para evitar log-forging.

---

## 3. Token: obtenerlo y rotarlo

El token de instalación vive en `%LOCALAPPDATA%\PrinklyPrint\auth.json` (256 bits,
base64url). Como tiene DACL owner-only, lo lee el **usuario dueño** del agente:

```powershell
(Get-Content "$env:LOCALAPPDATA\PrinklyPrint\auth.json" | ConvertFrom-Json).token
```

**Rotar el token** (ante sospecha de filtración): el efecto es que **todas** las
apps con token cacheado quedan invalidadas y tendrán que **re-parear**. Para los
orígenes que sigan en `allowed_origins`, el re-pareo es silencioso (no vuelve a
pedir diálogo); para el resto, vuelve a pedir aprobación.

- Vía la acción de regenerar token en la UI (si está disponible en tu versión).
- Alternativa de emergencia: detené el agente, borrá `auth.json` y reiniciá — el
  agente genera un token nuevo en el próximo arranque. (Esto rota el token; no
  toca `allowed_origins`.)

La rotación emite el evento `token_rotated` (1006).

---

## 4. Troubleshooting de impresión

Antes de imprimir, el agente hace un **pre-flight check** de la impresora
(`CheckReady`): si no existe, no hay default, o está en un estado bloqueante (sin
papel, sin tinta, offline, puerta abierta, etc.), el job falla rápido con un
mensaje claro en vez de gastar reintentos.

Pasos:

1. **Estado del job:** `GET /jobs/{id}` (o `GET /jobs` con filtros `status`,
   `limit`, `offset`). Mirá `status`, `last_error` y `sumatra_log`.
2. **`status=failed` con `last_error` de impresora no lista:** resolvé el estado
   físico (papel/tinta/online) y reintentá el job (`POST /jobs/{id}/retry`).
3. **Timeouts:** SumatraPDF se mata si excede el timeout de impresión; el job
   queda `failed` con el detalle. Revisá `sumatra_log` (truncado y saneado a propósito).
4. **Cola llena:** si hay 1000+ jobs en estado `queued`, `POST /print` responde
   `429 queue_full`. Drená/depurá la cola y reintentá.
5. **Reinicio del agente con jobs a medias:** al arrancar, los jobs que habían
   quedado en `printing` se revierten automáticamente a `queued` (recovery) y se
   reintentan.

---

## 5. Procedimiento: "los PDFs no se descifran" / jobs fallan tras un cambio

**Causa más probable: cambió la identidad de ejecución del agente.** El cifrado en
reposo usa DPAPI en scope de usuario; si el agente ahora corre bajo una cuenta
distinta a la que cifró los PDFs (p. ej. pasó a servicio SYSTEM, o cambió la
cuenta), `CryptUnprotectData` falla y esos jobs no se pueden imprimir.

Diagnóstico y remediación:

1. Confirmá **bajo qué cuenta** corre el proceso `prinklyprint.exe` (Administrador
   de tareas → Detalles → columna "Nombre de usuario").
2. Compará con la cuenta que venía usando el agente (la dueña de
   `%LOCALAPPDATA%\PrinklyPrint`).
3. Si difieren: volvé a la cuenta original. Los jobs cifrados por la cuenta
   anterior **no** se podrán recuperar bajo la nueva (es esperado).
4. Para evitarlo a futuro: no cambies la cuenta con jobs en reposo; drená la cola
   antes de cualquier cambio de identidad. Ver `docs/DEPLOYMENT_HARDENING.md`
   (Identidad de ejecución).

---

## 6. Backup y restore

- **Sí respaldar:** `config.yaml` (defaults + `allowed_origins`). Es portable
  entre equipos.
- **No respaldar / no restaurar en otra máquina:** `auth.json` (el token) y
  `pdfs\*.pdf.enc`. El token es por instalación; restaurarlo en otra máquina es
  una mala práctica de seguridad. Los `.pdf.enc` están atados por DPAPI al
  usuario/equipo de origen y **no se descifran** en otro lado.

Procedimiento de restore de config:

1. Detené el agente.
2. Copiá el `config.yaml` respaldado a `%LOCALAPPDATA%\PrinklyPrint\`.
3. Reiniciá el agente.
4. Si por algún motivo se restauró también `auth.json` desde otra máquina,
   **rotá el token** (sección 3) para no reutilizar un secreto trasplantado.

---

## 7. Primeros pasos ante un incidente

Cuando se sospecha abuso del agente (impresiones no autorizadas, accesos
rechazados anómalos):

1. **Contener:** si hace falta cortar el acceso de una app puntual, quitá su
   origen de `allowed_origins` (revoca de inmediato). Si se sospecha filtración
   del token, **rotá el token** (sección 3).
2. **Recolectar evidencia:** exportá los eventos `auth_failure` (1001),
   `pairing_*` (1002/1003) y `print_enqueued` (1004) del Event Log, más el log de
   archivo del período. El SIEM corporativo debería tenerlos centralizados.

   ```powershell
   Get-WinEvent -ProviderName PrinklyPrint |
     Where-Object Id -in 1001,1002,1003,1004,1006 |
     Export-Csv -NoTypeInformation incidente_prinklyprint.csv
   ```
3. **Analizar:** correlacioná `print_enqueued` con el `Origin` y el `filename`
   (saneados) para entender qué se imprimió y desde dónde. Un pico de
   `auth_failure` sin `Origin` sugiere un proceso local probando el loopback; con
   `Origin` sugiere una app/navegador.
4. **Handoff:** escalá al equipo de respuesta a incidentes de la organización con
   la evidencia recolectada. Los tiempos de respuesta y la cadena de
   escalamiento los define la organización (este runbook no fija SLAs).

---

## 8. Apagado / reinicio ordenado

El agente maneja `SIGINT`/`SIGTERM` (y el cierre desde la UI/bandeja) con un
apagado ordenado: drena la cola con timeout, cierra la base SQLite y los logs, y
libera el mutex de instancia única (`Global\PrinklyPrintSingletonMutex_v1`). El
instalador, al actualizar, cierra la instancia previa usando ese mismo mutex.
