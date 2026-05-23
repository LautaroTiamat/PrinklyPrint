// Package i18n contiene las traducciones de TODA la UI nativa.
// A diferencia de la versión Wails (donde i18n vivía en TypeScript), acá vive
// en Go puro y se invoca con T(lang, key).
package i18n

import "strings"

type Lang string

const (
	ES Lang = "es"
	EN Lang = "en"
	PT Lang = "pt"
)

var Supported = []Lang{ES, EN, PT}

func IsSupported(l string) bool {
	for _, s := range Supported {
		if string(s) == l {
			return true
		}
	}
	return false
}

// dict[key][lang] = traducción
var dict = map[string]map[Lang]string{
	// App
	"app.title":         {ES: "PrinklyPrint", EN: "PrinklyPrint", PT: "PrinklyPrint"},
	"app.loading":       {ES: "Cargando…", EN: "Loading…", PT: "Carregando…"},
	"app.save":          {ES: "Guardar", EN: "Save", PT: "Salvar"},
	"app.cancel":        {ES: "Cancelar", EN: "Cancel", PT: "Cancelar"},
	"app.close":         {ES: "Cerrar", EN: "Close", PT: "Fechar"},
	"app.refresh":       {ES: "Refrescar", EN: "Refresh", PT: "Atualizar"},
	"app.ok":            {ES: "OK", EN: "OK", PT: "OK"},
	"app.yes":           {ES: "Sí", EN: "Yes", PT: "Sim"},
	"app.no":            {ES: "No", EN: "No", PT: "Não"},
	"app.error":         {ES: "Error", EN: "Error", PT: "Erro"},

	// Tabs
	"tab.queue":          {ES: "Cola", EN: "Queue", PT: "Fila"},
	"tab.print_settings": {ES: "Impresión", EN: "Printing", PT: "Impressão"},
	"tab.general":        {ES: "General", EN: "General", PT: "Geral"},

	// Queue
	"queue.filter":     {ES: "Filtro:", EN: "Filter:", PT: "Filtro:"},
	"queue.col.date":   {ES: "Fecha", EN: "Date", PT: "Data"},
	"queue.col.file":   {ES: "Archivo", EN: "File", PT: "Arquivo"},
	"queue.col.printer": {ES: "Impresora", EN: "Printer", PT: "Impressora"},
	"queue.col.status": {ES: "Estado", EN: "Status", PT: "Estado"},
	"queue.col.attempts": {ES: "Intentos", EN: "Attempts", PT: "Tentativas"},
	"queue.purge":      {ES: "Limpiar histórico", EN: "Clear history", PT: "Limpar histórico"},
	"queue.purge_confirm": {ES: "¿Borrar el histórico de jobs completados, fallidos y cancelados?", EN: "Delete history of done, failed and cancelled jobs?", PT: "Excluir histórico de jobs concluídos, com falha e cancelados?"},
	"queue.detail":     {ES: "Detalle", EN: "Detail", PT: "Detalhe"},
	"queue.retry":      {ES: "Reintentar", EN: "Retry", PT: "Repetir"},
	"queue.cancel_job": {ES: "Cancelar job", EN: "Cancel job", PT: "Cancelar job"},
	"queue.empty":      {ES: "Sin jobs.", EN: "No jobs.", PT: "Sem jobs."},

	// Status (compartido con el filtro de la cola, la columna "Estado" de la
	// tabla y el detalle del job). Primera letra en mayúscula para que se vea
	// consistente como ítems de lista / labels.
	"status.all":       {ES: "Todos", EN: "All", PT: "Todos"},
	"status.queued":    {ES: "En cola", EN: "Queued", PT: "Na fila"},
	"status.printing":  {ES: "Imprimiendo", EN: "Printing", PT: "Imprimindo"},
	"status.done":      {ES: "Completado", EN: "Done", PT: "Concluído"},
	"status.failed":    {ES: "Fallido", EN: "Failed", PT: "Falhou"},
	"status.cancelled": {ES: "Cancelado", EN: "Cancelled", PT: "Cancelado"},

	// Detalle del job
	"detail.title":       {ES: "Detalle del job", EN: "Job detail", PT: "Detalhe do job"},
	"detail.id":          {ES: "ID:", EN: "ID:", PT: "ID:"},
	"detail.state":       {ES: "Estado:", EN: "State:", PT: "Estado:"},
	"detail.created":     {ES: "Creado:", EN: "Created:", PT: "Criado:"},
	"detail.completed":   {ES: "Completado:", EN: "Completed:", PT: "Concluído:"},
	"detail.printer":     {ES: "Impresora:", EN: "Printer:", PT: "Impressora:"},
	"detail.last_error":  {ES: "Último error", EN: "Last error", PT: "Último erro"},
	"detail.sumatra":     {ES: "Salida SumatraPDF", EN: "SumatraPDF output", PT: "Saída SumatraPDF"},
	"detail.first_try":   {ES: "al primer intento", EN: "on first try", PT: "na primeira tentativa"},

	// Print settings
	"ps.default_printer": {ES: "Impresora default:", EN: "Default printer:", PT: "Impressora padrão:"},
	"ps.default_system":  {ES: "(default del sistema)", EN: "(system default)", PT: "(padrão do sistema)"},
	"ps.paper_size":      {ES: "Tamaño de papel:", EN: "Paper size:", PT: "Tamanho do papel:"},
	"ps.paper.custom":    {ES: "Personalizado", EN: "Custom", PT: "Personalizado"},
	"ps.orientation":     {ES: "Orientación:", EN: "Orientation:", PT: "Orientação:"},
	"ps.portrait":        {ES: "Vertical", EN: "Portrait", PT: "Retrato"},
	"ps.landscape":       {ES: "Horizontal", EN: "Landscape", PT: "Paisagem"},
	"ps.color":           {ES: "Color", EN: "Color", PT: "Cor"},
	"ps.duplex":          {ES: "Dúplex:", EN: "Duplex:", PT: "Duplex:"},
	"ps.duplex.none":     {ES: "No", EN: "No", PT: "Não"},
	"ps.duplex.long":     {ES: "Lado largo", EN: "Long edge", PT: "Borda longa"},
	"ps.duplex.short":    {ES: "Lado corto", EN: "Short edge", PT: "Borda curta"},
	"ps.scale":           {ES: "Escala:", EN: "Scale:", PT: "Escala:"},
	"ps.scale.fit":       {ES: "Ajustar", EN: "Fit", PT: "Ajustar"},
	"ps.scale.shrink":    {ES: "Reducir si no entra", EN: "Shrink to fit", PT: "Reduzir se não couber"},
	"ps.scale.noscale":   {ES: "Sin escalar", EN: "No scale", PT: "Sem escalar"},
	"ps.print_test":      {ES: "Imprimir página de prueba", EN: "Print test page", PT: "Imprimir página de teste"},
	"ps.test_queued":     {ES: "Página de prueba encolada", EN: "Test page queued", PT: "Página de teste na fila"},
	"ps.printers_title":  {ES: "Impresoras del sistema", EN: "System printers", PT: "Impressoras do sistema"},

	// Printer status
	"pstatus.ready":             {ES: "Lista", EN: "Ready", PT: "Pronta"},
	"pstatus.busy":              {ES: "Ocupada", EN: "Busy", PT: "Ocupada"},
	"pstatus.printing":          {ES: "Imprimiendo", EN: "Printing", PT: "Imprimindo"},
	"pstatus.toner_low":         {ES: "Poca tinta", EN: "Toner low", PT: "Pouca tinta"},
	"pstatus.no_toner":          {ES: "Sin tinta", EN: "Out of toner", PT: "Sem tinta"},
	"pstatus.paper_jam":         {ES: "Papel atascado", EN: "Paper jam", PT: "Papel atolado"},
	"pstatus.paper_out":         {ES: "Sin papel", EN: "Out of paper", PT: "Sem papel"},
	"pstatus.door_open":         {ES: "Tapa abierta", EN: "Door open", PT: "Tampa aberta"},
	"pstatus.offline":           {ES: "Desconectada", EN: "Offline", PT: "Desconectada"},
	"pstatus.error":             {ES: "Error", EN: "Error", PT: "Erro"},
	"pstatus.paused":            {ES: "En pausa", EN: "Paused", PT: "Pausada"},
	"pstatus.user_intervention": {ES: "Requiere acción", EN: "User intervention", PT: "Requer ação"},

	// General
	"gen.language":          {ES: "Idioma:", EN: "Language:", PT: "Idioma:"},
	"gen.appearance":        {ES: "Apariencia", EN: "Appearance", PT: "Aparência"},
	"gen.startup_title":     {ES: "Inicio del sistema", EN: "System startup", PT: "Início do sistema"},
	"gen.autostart":         {ES: "Iniciar PrinklyPrint cuando inicie Windows", EN: "Start PrinklyPrint when Windows starts", PT: "Iniciar PrinklyPrint quando o Windows iniciar"},
	"gen.autostart_help":    {ES: "Recomendado para que el agente esté siempre disponible cuando la web cliente intente imprimir.", EN: "Recommended so the agent is always available when the client web tries to print.", PT: "Recomendado para que o agente esteja sempre disponível quando a web do cliente tentar imprimir."},
	"gen.http_server":       {ES: "Servidor HTTP", EN: "HTTP server", PT: "Servidor HTTP"},
	"gen.port":              {ES: "Puerto:", EN: "Port:", PT: "Porta:"},
	"gen.cors_title":        {ES: "Orígenes CORS permitidos", EN: "Allowed CORS origins", PT: "Origens CORS permitidas"},
	"gen.cors_add":          {ES: "Agregar", EN: "Add", PT: "Adicionar"},
	"gen.cors_remove":       {ES: "Quitar", EN: "Remove", PT: "Remover"},
	"gen.cors_allow_any":    {ES: "Permitir cualquier origen (no recomendado)", EN: "Allow any origin (not recommended)", PT: "Permitir qualquer origem (não recomendado)"},
	"gen.queue_title":       {ES: "Cola", EN: "Queue", PT: "Fila"},
	"gen.max_retries":       {ES: "Máximo de reintentos:", EN: "Max retries:", PT: "Máx. tentativas:"},
	"gen.retention_days":    {ES: "Días de retención:", EN: "Retention days:", PT: "Dias de retenção:"},
	"gen.info_title":        {ES: "Información", EN: "Information", PT: "Informações"},
	"gen.version":           {ES: "Versión:", EN: "Version:", PT: "Versão:"},
	"gen.machine_id":        {ES: "Machine ID:", EN: "Machine ID:", PT: "ID da máquina:"},
	"gen.data_dir":          {ES: "Carpeta de datos:", EN: "Data folder:", PT: "Pasta de dados:"},
	"gen.author":            {ES: "Autor:", EN: "Author:", PT: "Autor:"},
	"gen.github":            {ES: "GitHub:", EN: "GitHub:", PT: "GitHub:"},
	"gen.open_logs":         {ES: "Abrir carpeta de logs", EN: "Open logs folder", PT: "Abrir pasta de logs"},
	"gen.shutdown":          {ES: "Cerrar PrinklyPrint", EN: "Shut down PrinklyPrint", PT: "Encerrar PrinklyPrint"},
	"gen.shutdown_confirm":  {ES: "¿Cerrar PrinklyPrint?\n\nEl agente se detendrá y la web no podrá imprimir hasta que lo vuelvas a iniciar.", EN: "Shut down PrinklyPrint?\n\nThe agent will stop and your web won't be able to print until you launch it again.", PT: "Encerrar PrinklyPrint?\n\nO agente será interrompido e a web não poderá imprimir até reiniciá-lo."},

	// Tray
	"tray.open":        {ES: "Abrir PrinklyPrint", EN: "Open PrinklyPrint", PT: "Abrir PrinklyPrint"},
	"tray.queue":       {ES: "Ver cola de impresión", EN: "View print queue", PT: "Ver fila de impressão"},
	"tray.settings":    {ES: "Configuración", EN: "Settings", PT: "Configurações"},
	"tray.pause":       {ES: "Pausar impresión", EN: "Pause printing", PT: "Pausar impressão"},
	"tray.resume":      {ES: "Reanudar impresión", EN: "Resume printing", PT: "Resume printing"},
	"tray.quit":        {ES: "Salir", EN: "Quit", PT: "Sair"},
	"tray.tooltip_ready":   {ES: "PrinklyPrint — listo", EN: "PrinklyPrint — ready", PT: "PrinklyPrint — pronto"},
	"tray.tooltip_busy":    {ES: "PrinklyPrint — procesando cola", EN: "PrinklyPrint — processing queue", PT: "PrinklyPrint — processando fila"},
	"tray.tooltip_failed":  {ES: "PrinklyPrint — hay jobs fallidos en las últimas 24h", EN: "PrinklyPrint — failed jobs in the last 24h", PT: "PrinklyPrint — jobs com falha nas últimas 24h"},

	// Already running
	"running.title": {ES: "PrinklyPrint", EN: "PrinklyPrint", PT: "PrinklyPrint"},
	"running.body":  {ES: "PrinklyPrint ya está corriendo en esta computadora.\n\nBuscá el ícono en la bandeja del sistema.", EN: "PrinklyPrint is already running on this computer.\n\nLook for the icon in the system tray.", PT: "O PrinklyPrint já está rodando neste computador.\n\nProcure o ícone na bandeja do sistema."},
}

// T traduce una clave al idioma dado. Si el idioma no tiene la clave, cae a EN.
// Si la clave no existe, devuelve la clave misma (útil para detectar typos).
// Soporta sustitución simple {var}: T(lang, "k", "var", "valor").
func T(lang Lang, key string, kv ...string) string {
	entry, ok := dict[key]
	if !ok {
		return key
	}
	s, ok := entry[lang]
	if !ok {
		s = entry[EN]
		if s == "" {
			return key
		}
	}
	for i := 0; i+1 < len(kv); i += 2 {
		s = strings.ReplaceAll(s, "{"+kv[i]+"}", kv[i+1])
	}
	return s
}
