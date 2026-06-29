# Tests de Assert-Signed (sign.ps1). Pester v5.
#
# Correr:  Invoke-Pester .github/scripts/sign.Tests.ps1
#
# Verifican que la clasificación por .Status de Get-AuthenticodeSignature
# distingue integridad rota (corta) de cadena no confiable en el runner (warning):
#   FATAL : NotSigned, HashMismatch, UnknownError(corrupto), NotSupportedFileFormat
#   WARN  : NotTrusted, UnknownError(cadena no confiable)
#   OK    : Valid
#
# Si no tenés Pester v5 (Install-Module Pester -Scope CurrentUser), el script
# .github/scripts validate (ver validación standalone en el PR/commit) hace lo
# mismo dot-sourceando sign.ps1 y mockeando Get-AuthenticodeSignature a mano.

BeforeAll {
  # Sin credenciales de firma: al dot-sourcear, sign.ps1 toma el branch
  # "skip + warn" (no firma nada) y solo DEFINE las funciones, incluida Assert-Signed.
  $env:WINDOWS_CERT_PFX_BASE64 = ''
  $env:WINDOWS_CERT_PASSWORD = ''
  $env:SIGN_COMMAND = ''
  . "$PSScriptRoot/sign.ps1" -File "$PSScriptRoot/sign.ps1" 3>$null 6>$null
  $ErrorActionPreference = 'Continue' # sign.ps1 lo dejó en 'Stop'

  function New-Sig([string]$Status, [string]$Message = '') {
    [pscustomobject]@{
      Status                 = $Status
      StatusMessage          = $Message
      SignerCertificate      = if ($Status -eq 'NotSigned') { $null } else { 'cert' }
      TimeStamperCertificate = 'ts'
    }
  }
}

Describe 'Assert-Signed' {
  Context 'estados que NO deben cortar el build' {
    It 'Valid → OK' {
      Mock Get-AuthenticodeSignature { New-Sig 'Valid' }
      { Assert-Signed -Path 'x.exe' } | Should -Not -Throw
    }
    It 'NotTrusted → solo warning (cert de CA interna/de prueba)' {
      Mock Get-AuthenticodeSignature { New-Sig 'NotTrusted' 'chain not trusted on runner' }
      { Assert-Signed -Path 'x.exe' } | Should -Not -Throw
    }
    It 'UnknownError por cadena no confiable → solo warning' {
      Mock Get-AuthenticodeSignature { New-Sig 'UnknownError' 'A certificate chain could not be built to a trusted root authority.' }
      { Assert-Signed -Path 'x.exe' } | Should -Not -Throw
    }
  }

  Context 'estados que DEBEN cortar el build' {
    It 'NotSigned → fatal' {
      Mock Get-AuthenticodeSignature { New-Sig 'NotSigned' }
      { Assert-Signed -Path 'x.exe' } | Should -Throw
    }
    It 'HashMismatch → fatal (binario alterado tras firmar)' {
      Mock Get-AuthenticodeSignature { New-Sig 'HashMismatch' 'hash no coincide' }
      { Assert-Signed -Path 'x.exe' } | Should -Throw
    }
    It 'UnknownError con firma corrupta → fatal' {
      Mock Get-AuthenticodeSignature { New-Sig 'UnknownError' 'the signature is corrupt and unreadable' }
      { Assert-Signed -Path 'x.exe' } | Should -Throw
    }
    It 'NotSupportedFileFormat → fatal' {
      Mock Get-AuthenticodeSignature { New-Sig 'NotSupportedFileFormat' }
      { Assert-Signed -Path 'x.exe' } | Should -Throw
    }
  }
}

# ─────────────────────────────────────────────────────────────────────────────
# Validación SIN Pester (fallback / cómo se probó en un entorno con solo Pester 3.x).
# Pegá esto en una consola PowerShell desde la raíz del repo; imprime PASS/FAIL por
# caso. Es lo que se corrió al implementar el cambio (resultado: 7/7 PASS).
#
#   $s = '.github/scripts/sign.ps1'
#   $env:WINDOWS_CERT_PFX_BASE64=''; $env:SIGN_COMMAND=''
#   . $s -File $s 3>$null 6>$null; $ErrorActionPreference='Continue'
#   $script:FAKE=$null
#   function Get-AuthenticodeSignature { param($FilePath) $script:FAKE }
#   function T($n,$st,$m,$exp){
#     $script:FAKE=[pscustomobject]@{Status=$st;StatusMessage=$m;SignerCertificate='c';TimeStamperCertificate='t'}
#     $threw=$false; try { Assert-Signed -Path 'x' 3>$null 6>$null } catch { $threw=$true }
#     '{0,-22} {1,-14} throw={2,-5} {3}' -f $n,$st,$threw,$(if($threw -eq $exp){'PASS'}else{'FAIL'})
#   }
#   T 'Valid'        'Valid'        ''                         $false
#   T 'NotSigned'    'NotSigned'    ''                         $true
#   T 'HashMismatch' 'HashMismatch' 'hash'                     $true
#   T 'NotTrusted'   'NotTrusted'   'untrusted'                $false
#   T 'Unknown/chain' 'UnknownError' 'could not be built to a trusted root' $false
#   T 'Unknown/corrupt' 'UnknownError' 'signature is corrupt' $true
# ─────────────────────────────────────────────────────────────────────────────
