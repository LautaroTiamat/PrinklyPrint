# Firma Authenticode de un archivo (.exe / instalador) — parametrizable.
#
# Modos (en orden de prioridad):
#   1. PFX desde secret: si WINDOWS_CERT_PFX_BASE64 está seteado, firma con
#      signtool usando ese certificado (.pfx en base64) y WINDOWS_CERT_PASSWORD.
#   2. Servicio externo / HSM: si SIGN_COMMAND está seteado, ejecuta ese comando
#      reemplazando el placeholder {FILE} por la ruta del archivo. Sirve para
#      firmar con Azure Trusted Signing, un HSM en red, smctl de DigiCert, etc.
#   3. Sin credenciales: NO falla — avisa y omite la firma (warn + skip). Esto
#      permite que builds de prueba / forks pasen; el release real SÍ debe firmar.
#
# Timestamp RFC3161 obligatorio cuando se firma (la firma sobrevive al
# vencimiento del certificado). Servidor configurable vía TIMESTAMP_URL.
#
# Verificación: confirmamos que la firma quedó INCRUSTADA (Get-AuthenticodeSignature
# con SignerCertificate presente). NO gateamos contra la cadena de confianza a una
# raíz pública (signtool verify /pa), porque un cert interno/de empresa/de prueba
# no encadena a una raíz instalada en el runner y haría fallar el build aunque la
# firma sea correcta. El status de cadena se reporta como advertencia.
#
# Uso:  pwsh .github/scripts/sign.ps1 -File dist/prinklyprint.exe

param(
  [Parameter(Mandatory = $true)][string]$File
)

$ErrorActionPreference = 'Stop'

if (-not (Test-Path $File)) {
  throw "sign.ps1: no existe el archivo a firmar: $File"
}

$timestamp = $env:TIMESTAMP_URL
if ([string]::IsNullOrWhiteSpace($timestamp)) {
  $timestamp = 'http://timestamp.digicert.com'
}

function Find-SignTool {
  # signtool viene con el Windows SDK; no está en PATH por defecto.
  $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  $candidates = Get-ChildItem -Path "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\signtool.exe" -ErrorAction SilentlyContinue
  if ($candidates) {
    # Elegimos la versión MÁS NUEVA del SDK ordenando por número de versión real
    # (el segmento ...\bin\<version>\x64\), no por string (10.0.9 > 10.0.22621 como string).
    $best = $candidates | Sort-Object -Property @{ Expression = {
      $seg = ($_.FullName -split '[\\/]')[-3]
      try { [version]$seg } catch { [version]'0.0' }
    } } -Descending | Select-Object -First 1
    return $best.FullName
  }
  throw "sign.ps1: no encontré signtool.exe (Windows SDK)."
}

# Clasifica el resultado de Get-AuthenticodeSignature por su .Status y decide si
# corta el build o solo advierte. La idea es distinguir "integridad comprometida"
# (bloquear) de "cadena no verificable en ESTE runner" (aceptable).
#
#   FATAL (throw → corta el build):
#     - NotSigned    : la firma no se aplicó.
#     - HashMismatch : firmado, pero el binario fue ALTERADO después de firmar
#                      (integridad rota). Este es el caso clave a cazar.
#     - Otros estados de firma inválida/ilegible (NotSupportedFileFormat, etc., y
#       UnknownError cuando el StatusMessage indica firma corrupta).
#   WARNING (no corta):
#     - NotTrusted, o UnknownError por cadena no confiable en el runner: esperado
#       con una CA interna/de prueba que NO está en el trust store del runner. La
#       confianza real la valida Windows en el endpoint contra la CA interna
#       (desplegada por GPO), así que acá no debe ser fatal.
#   OK:
#     - Valid : firma presente y cadena confiable.
function Assert-Signed {
  param([string]$Path)
  $sig = Get-AuthenticodeSignature -FilePath $Path
  $status = [string]$sig.Status
  $msg = $sig.StatusMessage

  switch ($status) {
    'Valid' {
      Write-Host "sign.ps1: '$Path' firmado y de confianza (Status=Valid)."
    }
    'NotSigned' {
      throw "sign.ps1: '$Path' NO quedó firmado (Status=NotSigned): la firma no se aplicó."
    }
    'HashMismatch' {
      # Firma presente pero el hash no coincide: el binario fue modificado DESPUÉS
      # de firmarse. Integridad comprometida → cortar SIEMPRE (no confundir con
      # NotTrusted, que es solo cadena no verificable).
      throw "sign.ps1: '$Path' con INTEGRIDAD ROTA (Status=HashMismatch): el binario fue alterado después de firmarse. $msg"
    }
    'NotTrusted' {
      # Cadena no confiable en el runner (cert de CA interna/de prueba). No es un
      # problema de integridad → solo advertencia.
      Write-Warning "sign.ps1: '$Path' firmado pero la cadena NO es confiable en el runner (Status=NotTrusted): esperado con CA interna/de prueba; la confianza la valida Windows en el endpoint. $msg"
    }
    'UnknownError' {
      # Ambiguo: Get-AuthenticodeSignature usa UnknownError tanto para "no se pudo
      # construir la cadena hasta una raíz de confianza" (→ warning) como para una
      # firma corrupta/ilegible (→ fatal). Desambiguamos por el StatusMessage; ante
      # duda preferimos warning para no romper el caso de cert interno (NotSigned y
      # HashMismatch ya se cortaron en sus propias ramas).
      if ($msg -match 'corrupt|tamper|ilegible|illegible|unreadable|malformed|cannot be read|no se pudo leer|bad signature') {
        throw "sign.ps1: '$Path' con firma corrupta/ilegible (Status=UnknownError): $msg"
      }
      Write-Warning "sign.ps1: '$Path' firmado; cadena no verificable en el runner (Status=UnknownError, probable raíz no confiable): $msg"
    }
    default {
      # Cualquier otro estado (NotSupportedFileFormat, Incompatible, …) es firma
      # inválida → cortar.
      throw "sign.ps1: '$Path' con firma inválida (Status=$status): $msg"
    }
  }

  # Aviso ortogonal (no corta): si llegamos hasta acá la firma está presente
  # (Valid / NotTrusted / UnknownError no-fatal); avisamos si falta el timestamp.
  if ($null -eq $sig.TimeStamperCertificate) {
    Write-Warning "sign.ps1: '$Path' firmado SIN timestamp RFC3161."
  }
}

if (-not [string]::IsNullOrWhiteSpace($env:WINDOWS_CERT_PFX_BASE64)) {
  Write-Host "sign.ps1: firmando '$File' con certificado .pfx (signtool)..."
  $signtool = Find-SignTool
  $pfxPath = Join-Path $env:RUNNER_TEMP 'codesign.pfx'
  [IO.File]::WriteAllBytes($pfxPath, [Convert]::FromBase64String($env:WINDOWS_CERT_PFX_BASE64))
  try {
    # Siempre pasamos /p (aunque el password esté vacío) para que signtool NO
    # abra un prompt interactivo que colgaría el runner si el .pfx tiene password
    # y el secret no se cargó.
    $pw = $env:WINDOWS_CERT_PASSWORD
    if ($null -eq $pw) { $pw = '' }
    & $signtool sign /fd sha256 /tr $timestamp /td sha256 /f $pfxPath /p $pw $File
    if ($LASTEXITCODE -ne 0) { throw "signtool sign salió con código $LASTEXITCODE" }
  }
  finally {
    Remove-Item $pfxPath -Force -ErrorAction SilentlyContinue
  }
  Assert-Signed -Path $File
}
elseif (-not [string]::IsNullOrWhiteSpace($env:SIGN_COMMAND)) {
  Write-Host "sign.ps1: firmando '$File' con servicio externo (SIGN_COMMAND)..."
  $cmd = $env:SIGN_COMMAND.Replace('{FILE}', $File)
  # Con $ErrorActionPreference='Stop', si SIGN_COMMAND es un cmdlet/función que
  # falla, propaga. No confiamos SOLO en $LASTEXITCODE (no lo setean los cmdlets):
  # la fuente de verdad es Assert-Signed, que confirma la firma incrustada.
  Invoke-Expression $cmd
  Assert-Signed -Path $File
}
else {
  Write-Warning "sign.ps1: sin credenciales de firma (WINDOWS_CERT_PFX_BASE64 ni SIGN_COMMAND). Se OMITE la firma de '$File'. El release de produccion DEBE estar firmado."
}
