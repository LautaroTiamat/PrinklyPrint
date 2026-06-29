# Guía de despliegue y hardening (on-premise)

Guía para desplegar PrinklyPrint en endpoints corporativos Windows de forma
segura. Neutra y basada en el comportamiento real del agente.

---

## 1. Ubicación de archivos y permisos esperados

El instalador copia el ejecutable a `Program Files` (escribible solo por
administradores). Los datos del agente viven en el perfil del usuario:

```
%LOCALAPPDATA%\PrinklyPrint\
├── auth.json            Token de instalación (256 bits)        → DACL owner-only
├── config.yaml          Configuración (incluye allowed_origins) → DACL owner-only
├── agent.db             Cola SQLite                             → DACL owner-only
│   ├── agent.db-wal     Sidecar WAL de SQLite                   → DACL owner-only
│   └── agent.db-shm     Sidecar shared-memory de SQLite         → DACL owner-only
├── pdfs\                PDFs en reposo, cifrados (<id>.pdf.enc) → dir DACL owner-only
├── print-tmp\           PDFs descifrados efímeros (al imprimir) → dir DACL owner-only
├── logs\                Logs de archivo (rotación diaria)       → dir DACL owner-only
└── bin\                 SumatraPDF.exe extraído (go:embed)
```

Ejecutable:

```
%ProgramFiles%\PrinklyPrint\prinklyprint.exe   (instalado por admin; no escribible por usuarios estándar)
```

**Permisos owner-only (DACL).** En Windows, los archivos/directorios sensibles
reciben una **DACL protegida** (sin herencia del padre) que concede Full Control
solo a: el **dueño** del objeto, **Local System (SY)** y **Administradores
locales (BA)**. Cualquier otro usuario queda sin acceso. La DACL exacta es:

```
D:PAI(A;;FA;;;OW)(A;;FA;;;SY)(A;;FA;;;BA)
```

En entornos no-Windows (solo desarrollo/CI) el equivalente es `0o600` para
archivos y `0o700` para directorios.

> Pendiente de validación en Windows real: que la DACL quede efectivamente
> owner-only. Verificable con `icacls <ruta>` (ver `docs/PENTEST_GUIDE.md`).

---

## 2. Identidad de ejecución (CRÍTICO)

El cifrado en reposo usa **DPAPI en scope de usuario**: la clave deriva del perfil
del usuario que corre el agente. De esto se desprende una regla operativa
**obligatoria**:

> **El agente DEBE correr siempre bajo el MISMO usuario que cifró los PDFs.**

Consecuencias:

- El agente está diseñado para correr **como el usuario interactivo**
  (`asInvoker`, sin elevación), arrancado por la entrada de autostart en
  `HKCU\...\Run`. Esta es la configuración soportada.
- **Instalarlo como servicio bajo la cuenta `SYSTEM`** (o bajo cualquier cuenta
  distinta a la que cifró los datos) **rompe el descifrado** de los jobs que
  quedaron en reposo: `CryptUnprotectData` fallará y esos jobs fallarán al
  imprimir. No es un bug: es el comportamiento esperado del scope de usuario.
- **Cambiar la cuenta de ejecución** después de que ya hay PDFs cifrados tiene el
  mismo efecto. Si necesitás cambiar de cuenta, primero drená la cola (que no
  queden jobs en reposo) o asumí que los pendientes no se podrán imprimir.
- El modo `--headless` (sin UI) es para arrancar el agente antes del login
  interactivo (p. ej. una tarea programada). Si lo usás con una cuenta de
  servicio, recordá que esa **misma** cuenta será la única que pueda descifrar lo
  que cifre. No mezcles identidades.

Síntoma típico de identidad equivocada: "los PDFs no se descifran / los jobs
fallan al imprimir tras un cambio de cuenta". Ver `docs/RUNBOOK.md`.

---

## 3. Configuración corporativa por GPO

### 3.1 Certificado / CA interna en el trust store
Si los artefactos se firman con una **CA interna**, desplegá el certificado raíz
(y los intermedios) en el **almacén de Entidades de certificación raíz de
confianza** de los endpoints vía GPO. Sin esto, la validación de la firma del
instalador/ejecutable marcará la cadena como **no confiable** aunque la firma sea
válida (la integridad del binario se verifica igual; lo que falla es la confianza
en la cadena). Ver `docs/PENTEST_GUIDE.md` (integridad de artefactos).

### 3.2 Registro del source del Event Log
Los eventos de seguridad se publican en el **Windows Event Log** (canal
Application, source `PrinklyPrint`). El **instalador ya registra el source**
automáticamente (corre elevado: ejecuta `prinklyprint.exe --register-eventlog`).
Si se instala de forma no estándar, registralo una vez **como administrador**:

```powershell
& "C:\Program Files\PrinklyPrint\prinklyprint.exe" --register-eventlog
```

Si el source no está registrado, el agente **sigue funcionando** y loguea solo a
archivo (con un warning). Para quitarlo: `--unregister-eventlog` (lo hace el
desinstalador).

### 3.3 Política de Local Network Access del navegador
Los navegadores basados en Chromium están incorporando avisos/permisos para que
una página pública acceda a direcciones de red local/loopback (Local Network
Access / Private Network Access). Para evitar el prompt en los endpoints,
**pre-aprobá por política** los orígenes internos que llaman al agente (por
ejemplo, mediante las políticas empresariales de Chrome/Edge que permiten el
acceso a recursos locales para una lista de sitios). Esto es complementario al
pairing del agente: el navegador deja pasar el request y el agente sigue
aplicando token + diálogo.

---

## 4. Postura de red

- **Solo loopback.** El agente escucha en `127.0.0.1:17777` (puerto configurable
  en la UI). No bindea a interfaces de red.
- **Sin conexiones salientes.** El agente no descarga nada en operación normal
  (no existe `pdf_url`).
- **Firewall:** no hay que abrir **ningún** puerto entrante ni saliente para el
  agente. Si una regla corporativa bloquea loopback (poco común), permití
  `127.0.0.1:17777` solo localmente.

---

## 5. Orígenes permitidos y flujo de pairing

`config.yaml` tiene la lista `allowed_origins` (visible y editable desde la UI,
pestaña General → Orígenes CORS). Es la **única fuente de verdad** de orígenes
autorizados a imprimir.

Flujo:

1. La app web llama a un endpoint sensible sin token → `401`.
2. La librería cliente llama a `POST /pair`.
3. Si el origen **ya está** en `allowed_origins` (o si el modo "permitir cualquier
   origen" está activo — ver más abajo), el agente devuelve el token **sin diálogo**.
4. Si es un origen **nuevo**, el agente muestra un **diálogo nativo** al operador
   (por defecto en "denegar"). Al aprobar, el origen se agrega a `allowed_origins`
   y se entrega el token.
5. Quitar un origen de la lista **revoca** su acceso de inmediato (el próximo
   request da `401` y se vuelve a pedir aprobación).

Para entornos `--headless` (sin UI para el diálogo): **pre-aprobá** los orígenes
internos agregándolos a `allowed_origins` antes de desplegar (o corré el agente
en modo interactivo una vez para aprobarlos). En headless, un origen no
pre-aprobado recibe `403 pairing_denied`.

### Modo "permitir cualquier origen" (solo por instalador)

Existe un modo que desactiva el diálogo para **todos** los orígenes (el token
sigue siendo obligatorio). Es **inseguro** y por eso **solo se habilita al
instalar**, no en runtime:

- **No** está en `config.yaml` ni en la UI (la UI lo muestra como **solo lectura**).
  Editar el yaml a mano no lo activa.
- La única fuente de verdad es una marca en el registro
  **`HKLM\Software\PrinklyPrint\AllowAnyOrigin`** (DWORD = 1), que solo un proceso
  elevado (el instalador) puede escribir. Un usuario estándar no puede tocar HKLM.
- Se habilita marcando la opción **"Permitir cualquier origen CORS (NO
  recomendado)"** en el instalador, o en instalación silenciosa/GPO con
  `PrinklyPrint-Setup.exe /VERYSILENT /ALLOWANYORIGIN=1`. El desinstalador limpia
  la marca; reinstalar sin la opción la desactiva.
- Si el agente arranca con este modo activo, emite un evento de seguridad
  **WARNING** al Event Log/SIEM (`insecure_mode_enabled`) para que quede
  registrado que el equipo corre en modo inseguro.

En general **no lo uses**: preferí una lista explícita de `allowed_origins`. Solo
tiene sentido en entornos de prueba acotados.

---

## 6. Rate limit de `POST /pair` (opcional, apagado por default)

El pareo (`POST /pair`) puede limitarse por tasa para mitigar intentos de fuerza
bruta, **sin tocar la impresión**:

- **Apagado por default.** Se activa desde la UI (pestaña General → "Límite de
  intentos de pareo") o en `config.yaml`:
  `pair_rate_limit_enabled` (bool), `pair_rate_limit_per_minute` (tasa sostenida,
  default 30), `pair_rate_limit_burst` (capacidad de ráfaga, default 10). Los
  valores se leen en vivo (no requieren reiniciar).
- Algoritmo **token bucket** (tasa + burst): tolera ráfagas legítimas y modera el
  ritmo sostenido. Al superarse, `/pair` responde `429 rate_limited` con
  `Retry-After`, y se registra un evento de seguridad en el SIEM.
- **No aplica a `/print`** (a propósito): la impresión ya está acotada por el
  límite de tamaño de body (50 MB) y la profundidad máxima de cola. El rate limit
  es exclusivo del pareo, que se llama una vez por origen (no por impresión), así
  que limitarlo no afecta el flujo de impresión productiva.

---

## 7. Checklist de despliegue endurecido

- [ ] Instalar con el instalador firmado, como administrador.
- [ ] CA interna desplegada por GPO (si se firma con CA propia).
- [ ] Verificar la firma del `.exe` e instalador (`docs/PENTEST_GUIDE.md`).
- [ ] Confirmar que el agente corre **como el usuario** (no SYSTEM) si se usan los
      datos en reposo.
- [ ] Pre-aprobar los orígenes internos en `allowed_origins` (sobre todo en
      `--headless`).
- [ ] Confirmar el registro del Event Log source y la recolección por el SIEM.
- [ ] Política de Local Network Access del navegador para los orígenes internos.
- [ ] Verificar DACL owner-only de la carpeta de datos (`icacls`).
