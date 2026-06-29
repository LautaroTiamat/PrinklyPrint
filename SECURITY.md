# Política de seguridad

## Reportar una vulnerabilidad

Agradecemos los reportes de seguridad responsables. **No abras un issue público**
para una vulnerabilidad.

- **Canal preferido:** usá *Report a vulnerability* en la pestaña **Security** del
  repositorio de GitHub (GitHub Security Advisories, privado). Esto crea un canal
  confidencial entre quien reporta y los mantenedores.
- **Qué incluir:** versión afectada, descripción, pasos de reproducción / PoC,
  impacto estimado y, si podés, una recomendación de remediación. La plantilla de
  `docs/PENTEST_GUIDE.md` (sección C.5) sirve de guía.
- **Qué esperar:** confirmaremos la recepción, evaluaremos el impacto y
  coordinaremos la divulgación una vez disponible una corrección. Pedimos no
  divulgar públicamente hasta entonces.

Por favor, limitá las pruebas a equipos propios o con autorización explícita. El
agente escucha solo en loopback, así que las pruebas son inherentemente locales.

## Versiones soportadas

Se da soporte de seguridad a la **última versión menor publicada**. Actualizá a la
release más reciente antes de reportar para descartar problemas ya corregidos.

| Versión | Soporte |
|---------|---------|
| 1.2.x   | ✅ |
| < 1.2   | ❌ (actualizá) |

## Resumen de la postura de seguridad

- **Solo loopback.** La API HTTP escucha exclusivamente en `127.0.0.1:17777`
  (binding explícito a loopback). No es accesible desde la red.
- **Sin conexiones salientes.** El agente solo imprime PDFs enviados inline
  (base64); no descarga desde URLs remotas. No existe la clase de vulnerabilidad
  SSRF.
- **Token + pairing con consentimiento.** Los endpoints sensibles exigen un token
  Bearer por instalación; un origen nuevo requiere la aprobación explícita del
  operador mediante un diálogo nativo. Quitar un origen de la lista revoca su
  acceso de inmediato.
- **Validación de entrada.** Las opciones de impresión se validan con whitelist
  antes de pasar a SumatraPDF (en particular `page_range`), evitando inyección de
  directivas.
- **Datos en reposo cifrados.** Los PDFs se guardan cifrados con DPAPI (scope de
  usuario) y, junto con el token, la cola y la config, con permisos owner-only
  (DACL protegida en Windows).
- **Logging a Windows Event Log.** Eventos de seguridad con IDs estables para
  recolección por un SIEM; el token nunca se loguea.
- **Cadena de suministro.** Releases con ejecutable e instalador firmados
  (Authenticode), attestation de procedencia, SBOM (CycloneDX), verificación del
  hash de las dependencias embebidas, y escaneos de SAST/SCA/secretos en CI.

Más detalle: [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md),
[`docs/DEPLOYMENT_HARDENING.md`](docs/DEPLOYMENT_HARDENING.md),
[`docs/PENTEST_GUIDE.md`](docs/PENTEST_GUIDE.md) y
[`docs/RUNBOOK.md`](docs/RUNBOOK.md).
