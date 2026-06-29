// Command prinklyprint es el ejecutable del agente de impresión PrinklyPrint.
//
// PrinklyPrint es un agente local que corre en la PC del operador, expone un
// servidor HTTP en 127.0.0.1:17777 al que su aplicación web puede mandarle
// PDFs, y los imprime silenciosamente usando SumatraPDF embebido.
//
// Arquitectura general:
//
//	┌────────────────────────────────────────────────────────────┐
//	│ PC del operador                                            │
//	│                                                            │
//	│  Navegador (web cliente) ◄──HTTP loopback──► prinklyprint   │
//	│                                                  │         │
//	│                                                  ▼         │
//	│                                          SumatraPDF.exe    │
//	│                                                  │         │
//	│                                                  ▼         │
//	│                                          Impresora local   │
//	└────────────────────────────────────────────────────────────┘
//
// El entry point hace lo mínimo: parsea flags, arma el contexto con manejo de
// SIGINT/SIGTERM, construye el [app.App] y delega todo en [app.App.Run].
//
// Modos de arranque:
//
//   - Sin flags (default): arranca todo — servidor HTTP, worker de cola,
//     ícono de bandeja y ventana de configuración nativa.
//
//   - Con --headless: NO arranca la UI (ni ventana ni tray). Se usa cuando el
//     agente corre como servicio del sistema (Task Scheduler con cuenta SYSTEM
//     en AtStartup), antes de que ningún usuario haya iniciado sesión.
//
//   - Con --version: imprime la versión inyectada en build time y termina.
//
// La variable [version] se inyecta vía ldflag durante la compilación:
//
//	go build -ldflags "-X main.version=0.1.0" .
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lautarotiamat/prinklyprint/internal/app"
	"github.com/lautarotiamat/prinklyprint/internal/seclog"
)

// version se inyecta en build time. En desarrollo queda como "dev".
var version = "dev"

// eventLogSource es el source del Windows Event Log que registra el instalador.
const eventLogSource = "PrinklyPrint"

func main() {
	headless := flag.Bool("headless", false, "Arranca sin UI (solo server + queue + tray)")
	showVersion := flag.Bool("version", false, "Muestra la versión y sale")
	// Registro del Event Log (requiere admin): lo corre el instalador elevado.
	registerEvt := flag.Bool("register-eventlog", false, "Registra el source del Event Log (requiere admin) y sale")
	unregisterEvt := flag.Bool("unregister-eventlog", false, "Quita el source del Event Log (requiere admin) y sale")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if *registerEvt {
		if err := seclog.RegisterEventSource(eventLogSource); err != nil {
			fmt.Fprintf(os.Stderr, "no se pudo registrar el Event Log: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Event Log source %q registrado.\n", eventLogSource)
		return
	}
	if *unregisterEvt {
		if err := seclog.UnregisterEventSource(eventLogSource); err != nil {
			fmt.Fprintf(os.Stderr, "no se pudo quitar el Event Log: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Event Log source %q quitado.\n", eventLogSource)
		return
	}

	// signal.NotifyContext nos da un context que se cancela cuando llega SIGINT
	// (Ctrl+C en consola) o SIGTERM (terminación del SO). El App.Run lo escucha
	// y dispara un apagado ordenado: drena la cola, cierra el server, persiste
	// estado, libera el mutex de instancia única, etc.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	a, err := app.New(app.Options{Version: version, Headless: *headless})
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	if err := a.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "exit: %v\n", err)
		os.Exit(1)
	}
}
