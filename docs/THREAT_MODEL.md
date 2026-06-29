# Modelo de amenazas — PrinklyPrint

Este documento describe los activos, los límites de confianza, las amenazas por
componente y los riesgos residuales del agente de impresión PrinklyPrint. Está
basado en el comportamiento real del código y se mantiene en lenguaje neutro
para un repositorio público.

PrinklyPrint es un agente local para Windows que expone una API HTTP **solo en
loopback** (`127.0.0.1:17777`, puerto configurable) y que imprime PDFs de forma
silenciosa con SumatraPDF. No es un servicio de red: nada de su superficie es
accesible desde fuera de la máquina.

---

## 1. Activos y su sensibilidad

| Activo | Dónde vive | Sensibilidad | Protección |
|--------|------------|--------------|------------|
| **PDFs en tránsito** | Body de `POST /print` (base64), en memoria | Alta (pueden contener datos de negocio) | Loopback (no sale de la máquina), token + pairing |
| **PDFs en reposo** | `%LOCALAPPDATA%\PrinklyPrint\pdfs\<id>.pdf.enc` | Alta | Cifrado DPAPI (scope usuario) + DACL owner-only |
| **PDF en claro temporal** | `%LOCALAPPDATA%\PrinklyPrint\print-tmp\<uuid>.pdf` (efímero, solo durante la impresión) | Alta | DACL owner-only + sobrescritura best-effort + borrado inmediato |
| **Token de instalación** | `%LOCALAPPDATA%\PrinklyPrint\auth.json` | Crítica (concede acceso a los endpoints sensibles) | 256 bits crypto/rand, DACL owner-only, comparación en tiempo constante, nunca se loguea |
| **Cola de jobs** | `%LOCALAPPDATA%\PrinklyPrint\agent.db` (SQLite, + `-wal`/`-shm`) | Media (metadata, nombres de archivo, errores) | DACL owner-only |
| **Configuración** | `%LOCALAPPDATA%\PrinklyPrint\config.yaml` | Media (incluye `allowed_origins`) | DACL owner-only, validación de rangos/enums |
| **Logs de seguridad** | Windows Event Log (canal Application, source `PrinklyPrint`) + archivo en `logs/` | Media | Saneo anti-forging, el SIEM corporativo es el registro de integridad |

Clasificación: los **PDFs** y el **token** son los activos de mayor valor. El
token es el único secreto que, si se filtra, permite encolar impresiones desde
un origen ya aprobado.

---

## 2. Límites de confianza

```
┌─────────────────────────── PC del operador (un solo usuario) ───────────────────────────┐
│                                                                                          │
│   Navegador / app web        HTTP loopback           Agente PrinklyPrint                 │
│   (origen aprobado) ───────► 127.0.0.1:17777 ───────► [CORS] ─► [token+origen] ─► handler│
│        │                     (nunca sale de la PC)                       │                │
│        │                                                                 ▼                │
│        │                                                   cola SQLite (cifrada en reposo)│
│        │                                                                 │                │
│        │                                                                 ▼                │
│        │                                                   SumatraPDF.exe (subproceso)    │
│        │                                                                 │                │
│        │                                                                 ▼                │
│        └────────────────────────────────────────────────────► Impresora local / red     │
│                                                                                          │
│   Event Log (Application) ──► recolectado por el SIEM corporativo (fuera de la PC)        │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

**Límites de confianza cruzados:**

1. **Navegador/app web → API loopback.** El navegador es semi-confiable: puede
   alojar una página maliciosa. La puerta es el **token Bearer** + el **diálogo
   de pairing** (consentimiento del operador), no CORS.
2. **API → SumatraPDF.** Las opciones de impresión controladas por el cliente se
   traducen a flags de línea de comando. La puerta es la **validación de entrada
   con whitelist** (especialmente `page_range`).
3. **Datos en reposo → otros usuarios / otras máquinas.** La puerta es **DPAPI
   (scope usuario)** + **DACL owner-only**.
4. **Proceso del agente → sistema.** El agente corre **como el usuario**
   (`asInvoker`, sin elevación) desde un directorio en `Program Files` (escribible
   solo por administradores).

**Postura de red:** el listener bindea explícitamente a `127.0.0.1` (loopback
IPv4). No hay binding a `0.0.0.0` ni a interfaces de red. El agente **no abre
ninguna conexión saliente** durante la operación normal. No se usa TLS porque el
tráfico nunca abandona la máquina.

---

## 3. Amenazas por componente (estilo STRIDE) y mitigación

### 3.1 Proceso local no-navegador que llama al loopback
- **Amenaza (Spoofing / Elevation):** cualquier proceso del equipo puede abrir
  un socket a `127.0.0.1:17777` e intentar imprimir o leer la cola.
- **Mitigación:** todos los endpoints sensibles exigen `Authorization: Bearer
  <token>`. Sin token válido → `401`, **tenga o no header `Origin`** (cierra el
  bypass clásico de "no mando Origin para evitar CORS"). El token vive en
  `auth.json` con DACL owner-only, así que solo el mismo usuario (o SYSTEM /
  administradores) puede leerlo.

### 3.2 Página web maliciosa en el navegador del operador
- **Amenaza (Spoofing / Tampering):** un sitio cualquiera intenta usar el agente
  para imprimir sin autorización.
- **Mitigación:** para obtener token, el origen tiene que pasar por `POST /pair`,
  que para un origen nuevo **abre un diálogo nativo** y requiere la aprobación
  explícita del operador. Sin aprobación no hay token. Al aprobar, el origen se
  agrega a `allowed_origins` (visible y editable por el operador). CORS es
  permisivo a propósito (ver 3.7), pero **no concede acceso**: el control es el
  token + el diálogo.

### 3.3 Exfiltración de PDFs en reposo
- **Amenaza (Information disclosure):** copiar los PDFs guardados a otro usuario o
  equipo para leerlos.
- **Mitigación:** los PDFs se guardan **cifrados** como `<id>.pdf.enc` con DPAPI
  en **scope de usuario** (`CryptProtectData`, sin `CRYPTPROTECT_LOCAL_MACHINE`).
  La clave deriva del perfil del usuario que corre el agente: un blob copiado a
  otro usuario/equipo **no se puede descifrar**. Además, el archivo y su
  directorio tienen DACL owner-only.

### 3.4 Inyección a SumatraPDF vía opciones de impresión
- **Amenaza (Tampering / Elevation):** manipular las opciones (`page_range`,
  `paper_size`, etc.) para inyectar directivas extra en `-print-settings` de
  SumatraPDF.
- **Mitigación:** validación de entrada con **whitelist estricta** antes de
  encolar. `page_range` solo admite `^[0-9,\- ]*$` (dígitos, comas, guiones,
  espacios): no puede introducir directivas alfabéticas (`color`, `landscape`,
  `paper=…`, etc.). `paper_size` admite solo `^[A-Za-z0-9 _-]+$` (máx. 64).
  `orientation`/`duplex`/`scale` son enums cerrados. `copies` ≤ 999.

### 3.5 SSRF / pivoteo a la red interna
- **Amenaza (originalmente):** pedirle al agente que descargue un PDF desde una
  URL arbitraria habría permitido inducirlo a contactar recursos internos.
- **Mitigación:** **eliminada por diseño.** El agente solo imprime PDFs enviados
  inline (`pdf_base64`); no existe descarga remota. El agente **no realiza
  ninguna conexión saliente**, de modo que la clase de vulnerabilidad SSRF no
  aplica.

### 3.6 Manipulación / borrado de evidencia (logs)
- **Amenaza (Repudiation / Tampering):** forjar o ensuciar las entradas de log
  para ocultar actividad, o inyectar líneas falsas a través de campos que
  controla el cliente (filename, Origin).
- **Mitigación:** los eventos de seguridad se emiten al **Windows Event Log**
  (recolectado centralmente por el SIEM, donde vive la integridad/retención
  real), además del archivo local complementario. Los valores controlados por el
  cliente se **sanean** (se reemplazan CR/LF y caracteres de control, se acota la
  longitud a 256) para evitar log-forging. El **token nunca** se escribe en logs.

### 3.7 CORS permisivo
- **Aclaración (no es una amenaza, es una decisión):** CORS refleja cualquier
  `Origin` y responde el preflight. Esto **no** otorga acceso: un origen sin
  token recibe `401`, y para conseguir token hay que pasar por el diálogo. Si
  CORS fuera estricto, el navegador bloquearía el preflight **antes** del `401` y
  el flujo de pairing (401 → `/pair` → diálogo) nunca podría arrancar para un
  origen nuevo.

### 3.8 Revocación de un origen
- **Amenaza (Elevation):** una app que ya tiene el token cacheado sigue
  imprimiendo después de que se le revocó el permiso.
- **Mitigación:** además del token, los requests del navegador (con `Origin`)
  exigen que el origen **siga** en `allowed_origins`. Quitar el origen de la
  lista revoca el acceso de inmediato (`401`), aunque la app conserve el token; la
  librería lo interpreta como "re-pareá" y el agente vuelve a pedir aprobación.

---

## 4. Riesgos residuales (declarados honestamente)

Estos riesgos se conocen y se aceptan; no hay mitigación perfecta dentro del
alcance del agente.

1. **Ventana de texto plano durante la impresión.** Para imprimir, el agente
   descifra el PDF a un archivo temporal en `print-tmp\`. Durante el breve lapso
   en que SumatraPDF lee ese archivo, el PDF está en claro en disco. Se mitiga
   con DACL owner-only + sobrescritura best-effort + borrado inmediato apenas
   SumatraPDF termina, y con un barrido de huérfanos al arrancar. **Limitación
   honesta:** la sobrescritura no es un borrado seguro en SSD (wear-leveling /
   copy-on-write pueden dejar copias del bloque). Quien tenga acceso físico al
   disco y a la clave de usuario podría, en teoría, recuperar residuos.

2. **Acople de DPAPI al usuario que corre el agente.** El cifrado en reposo está
   atado a la identidad del usuario. Si el agente se reconfigura para correr bajo
   **otra cuenta** (por ejemplo, como servicio SYSTEM), **no podrá descifrar** los
   PDFs cifrados por la cuenta anterior. Es un comportamiento esperado del scope
   de usuario; ver `docs/DEPLOYMENT_HARDENING.md` (Identidad de ejecución).

3. **Transporte HTTP (no HTTPS).** Aceptable porque es **loopback**: el tráfico
   entre la app web y el agente no sale de la máquina, por lo que no hay un
   tramo de red que interceptar. La comunicación de la app web con sus propios
   backends por la red interna de la organización está **fuera del alcance** del
   agente y su riesgo residual lo asume la organización.

4. **Confianza en el almacén de certificados del endpoint.** La verificación de
   la firma del instalador/ejecutable depende de que la CA correspondiente esté
   en el trust store del equipo. Si la organización usa una CA interna, debe
   desplegarla por GPO; de lo contrario la validación de la firma marcará la
   cadena como no confiable (ver `docs/DEPLOYMENT_HARDENING.md`).

5. **Disponibilidad del token para el propio usuario.** El token es legible por
   el usuario dueño (y por SYSTEM/Administradores, por la DACL). Comprometer la
   cuenta del usuario o un administrador local implica acceso al token. Es
   inherente a un secreto local por instalación; la mitigación operativa es rotar
   el token ante sospecha (ver `docs/RUNBOOK.md`).

---

## 5. Pendiente de validación en Windows real

Varias mitigaciones son Windows-only y, en este entorno de desarrollo (Linux/CI),
se ejercitan mediante stubs e interfaces. **Quedan pendientes de validación en un
Windows real**: el cifrado efectivo en disco (que `file`/un visor no reconozca un
`.pdf.enc` como PDF), la DACL owner-only (verificable con `icacls`), la aparición
de eventos en el Visor de eventos, y la firma con un certificado real. El detalle
de cómo verificarlas está en `docs/PENTEST_GUIDE.md`.
