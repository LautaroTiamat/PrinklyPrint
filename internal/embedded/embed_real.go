//go:build with_sumatra

package embedded

import _ "embed"

//go:embed SumatraPDF.exe
var SumatraPDF []byte
