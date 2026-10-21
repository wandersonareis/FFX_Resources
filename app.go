package main

import (
	"context"
	"ffxresources/backend/appupdater"
	"ffxresources/backend/common"
	"ffxresources/backend/interactions"
	"ffxresources/backend/loggingService"
	"ffxresources/backend/progress"
	"ffxresources/backend/services"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// App é o serviço de SESSÃO e CICLO DE VIDA: ciclo de vida do Wails, flag de
// edições não salvas e os poucos métodos que não pertencem a um domínio.
//
// No v2 ele era um god object com ~62 métodos bound de todos os domínios; no
// v3 cada domínio virou um service próprio (service_config.go, service_vbf.go,
// service_text.go, service_image.go) e só isto permanece aqui.
type App struct {
	noticationService services.INotificationService

	statusBarService services.IStatusBarService

	MetadataService *services.MetadataService

	// Serviços por domínio, registrados em appServices(). O ctx do v2 saiu: no
	// Wails v3 o app é alcançado por application.Get() e os serviços de ciclo
	// de vida recebem um context.Context próprio em ServiceStartup.
	config *ConfigService
	vbf    *VbfService
	text   *TextService
	image  *ImageService

	unsavedMu    sync.Mutex
	unsavedEdits bool
}

// NewApp cria o App e toda a árvore de dependências de uma vez.
//
// Construir aqui (e não em ServiceStartup) é o que permite registrar os
// serviços por domínio em application.New: no v3 os structs são criados antes
// de o app rodar, então precisam do MetadataService pronto na construção.
func NewApp() *App {
	notifier := services.NewEventNotifier()

	// Ponte de progresso: os serviços (export/extração) criam batches no
	// emitter INJETADO e publicam snapshots nos eventos do wails
	// (ProgressStart/Update/End) — sem global, sem canal de pacote.
	progressEmitter := progress.NewEmitter(notifier.NotifyWarn)

	// Ponte da barra de status: desalinhamentos (original vs tradução)
	// publicados pelos services chegam à barra de rodapé do frontend.
	statusSvc := services.NewStatusBarService()
	services.SetStatusBarService(statusSvc)

	// Um único MetadataService é compartilhado por todos os serviços de
	// domínio: ele carrega os caches de índice/imagens, e instâncias
	// separadas serviriam caches diferentes para o mesmo dado.
	meta := services.NewMetadataService(notifier, progressEmitter)

	return &App{
		noticationService: notifier,
		statusBarService:  statusSvc,
		MetadataService:   meta,
		config:            &ConfigService{notifier: notifier},
		vbf:               &VbfService{meta: meta},
		text:              &TextService{meta: meta},
		image:             &ImageService{meta: meta},
	}
}

// appServices devolve o App e os serviços por domínio para registrar em
// application.New. Cada struct registrado vira um módulo próprio de bindings
// no frontend (bindings/ffxresources/<service>).
//
// É função de pacote e não método: métodos exportados de structs registrados
// são expostos ao frontend, e registrar o app de novo a partir do JS não faz
// sentido.
func appServices(a *App) []application.Service {
	return []application.Service{
		application.NewService(a),
		application.NewService(a.config),
		application.NewService(a.vbf),
		application.NewService(a.text),
		application.NewService(a.image),
		application.NewService(&WindowService{}),
		application.NewService(&UpdateService{}),
	}
}

// ServiceStartup substitui o OnStartup do v2: roda durante app.Run(), antes
// do event loop. O ctx recebido é cancelado logo antes do shutdown.
func (a *App) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	// O arquivo de log nasce aqui: um por início de app, ao lado do
	// executável (logs/ffx-<início>.log) — antes de qualquer mensagem.
	// A linha abaixo garante conteúdo desde o primeiro instante (e deixa
	// o caminho do log à vista no próprio arquivo).
	loggingService.Init()
	loggingService.Info("FINAL FANTASY X and X-2 HD Remaster Resources Editor iniciado — log em %s", loggingService.LogDir())

	// Perform your setup here
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred:", err)

			l := loggingService.Get()
			l.Fatal().Caller(2).Err(err.(error)).Msg("panic occurred")
		}
	}()

	return nil
}

// shouldQuit substitui o OnBeforeClose do v2: roda a cada pedido de saída
// (fechar a última janela, Alt+F4 ou QuitApp — o v2 cobria os mesmos casos).
// Devolve true para PERMITIR a saída, o inverso do `prevent` do v2.
//
// O diálogo do v2 devolvia a resposta em runtime.MessageDialog; no v3
// Show() bloqueia e cada botão roda o próprio callback, então a decisão
// vira uma variável local que os handlers preenchem.
func (a *App) shouldQuit() bool {
	interactions.NewInteractionService().FFXAppConfig().ToJson()

	if !a.hasUnsavedEdits() {
		return true
	}

	app := application.Get()
	if app == nil {
		return false
	}

	// Atualização já baixada e em stage: o app está no meio de um restart
	// para aplicar a nova versão (updater.Restart → Quit) ou o usuário vai
	// aplicá-la em seguida. O diálogo de baixo CANCELARIA o restart — o
	// helper espera o pai sair e desiste após HelperReadyTimeout, deixando a
	// atualização parada — e uma saída em silêncio perderia edição sem
	// avisar. A pergunta muda, mantendo as mesmas três saídas.
	if appupdater.ReadyToApply() {
		return a.confirmUpdateRestart(app)
	}

	// Há edições não salvas: o texto mora no frontend, então "Salvar" emite
	// o evento SaveRequested (o frontend salva e chama QuitApp para fechar).
	// Sem confirmação explícita, a saída fica bloqueada (default false).
	allowQuit := false

	dlg := app.Dialog.Question().
		SetTitle("Alterações não salvas").
		SetMessage("Há textos editados não salvos. Deseja salvar antes de fechar?")
	save := dlg.AddButton("Salvar")
	discard := dlg.AddButton("Descartar")
	cancel := dlg.AddButton("Cancelar")
	dlg.SetDefaultButton(save)
	dlg.SetCancelButton(cancel)

	save.OnClick(func() {
		fmt.Println("Close answer: Salvar")
		allowQuit = false
		app.Event.Emit("SaveRequested")
	})
	discard.OnClick(func() {
		fmt.Println("Close answer: Descartar")
		a.SetUnsavedEdits(false)
		allowQuit = true
	})
	cancel.OnClick(func() {
		fmt.Println("Close answer: Cancelar")
		allowQuit = false
	})

	dlg.Show()
	return allowQuit
}

// confirmUpdateRestart é a pergunta de saída quando existe atualização em
// stage: o reinício vai trocar o binário, então as edições não salvas pedem
// decisão explícita. "Salvar e reiniciar" segue o mesmo circuito do diálogo
// normal (SaveRequested → frontend salva → QuitApp → segunda passada por
// shouldQuit não acha edição pendente e libera a saída); "Descartar"
// libera direto; "Cancelar" aborta o reinício — a atualização continua em
// stage e pode ser aplicada depois (banner "Reiniciar agora" ou menu).
func (a *App) confirmUpdateRestart(app *application.App) bool {
	msg := "O aplicativo vai reiniciar para concluir a atualização."
	if v := appupdater.ReadyVersion(); v != "" {
		msg = "O aplicativo vai reiniciar para aplicar a versão " + v + "."
	}
	msg += "\n\nAs alterações não salvas serão descartadas. O que fazer?"

	allowQuit := false
	dlg := app.Dialog.Question().
		SetTitle("Atualização pronta para aplicar").
		SetMessage(msg)
	save := dlg.AddButton("Salvar e reiniciar")
	discard := dlg.AddButton("Descartar e reiniciar")
	cancel := dlg.AddButton("Cancelar")
	dlg.SetDefaultButton(save)
	dlg.SetCancelButton(cancel)

	save.OnClick(func() {
		allowQuit = false
		app.Event.Emit("SaveRequested")
	})
	discard.OnClick(func() {
		a.SetUnsavedEdits(false)
		allowQuit = true
	})
	cancel.OnClick(func() {
		loggingService.Warn("update: reinício cancelado pelo usuário (versão %s segue em stage)", appupdater.ReadyVersion())
		allowQuit = false
	})

	dlg.Show()
	return allowQuit
}

// hasUnsavedEdits consulta a flag de edições pendentes (thread-safe).
func (a *App) hasUnsavedEdits() bool {
	a.unsavedMu.Lock()
	defer a.unsavedMu.Unlock()
	return a.unsavedEdits
}

// SetUnsavedEdits recebe do frontend o estado de edições não salvas.
func (a *App) SetUnsavedEdits(dirty bool) {
	a.unsavedMu.Lock()
	a.unsavedEdits = dirty
	a.unsavedMu.Unlock()
}

// QuitApp encerra a aplicação (usado após salvar no fluxo SaveRequested).
// app.Quit() atravessa shouldQuit, do mesmo jeito que o runtime.Quit do v2
// atravessava o beforeClose — com edições pendentes, o diálogo aparece de novo.
func (a *App) QuitApp() {
	if app := application.Get(); app != nil {
		app.Quit()
	}
}

// ServiceShutdown substitui o OnShutdown do v2: roda quando a saída já está
// confirmada, antes do shutdown dos services em ordem reversa.
func (a *App) ServiceShutdown() error {
	// Os .vbf abertos seguram handle de leitura: solta antes de sair.
	services.CloseVbfArchives()
	return nil
}

func (a *App) ReadFileAsString(file string) string {
	content, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	//fmt.Println(string(content))
	return string(content)
}

// pickDirectory abre o seletor nativo de PASTA com o título e a pasta
// inicial pedidos. Devolve "" fora do ciclo de vida do app e no cancelamento.
//
// O Wails v3 não tem OpenDirectoryDialog: escolher pasta é o OpenFileDialog
// com CanChooseDirectories(true). CanChooseFiles precisa ser false EXPLÍCITO
// porque o SetOptions do v3 liga de volta os dois desligados.
func pickDirectory(title, defaultDirectory string) string {
	app := application.Get()
	if app == nil {
		return ""
	}
	selection, err := app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:                title,
		Directory:            defaultDirectory,
		CanChooseFiles:       false,
		CanChooseDirectories: true,
	}).PromptForSingleSelection()
	if err != nil {
		return ""
	}
	return selection
}

// pickFile abre o seletor nativo de ARQUIVO com o título, a pasta inicial e
// os filtros pedidos (pares DisplayName/Pattern, como no v2). Devolve "" fora
// do ciclo de vida do app e no cancelamento.
func pickFile(title, defaultDirectory string, filters ...application.FileFilter) string {
	app := application.Get()
	if app == nil {
		return ""
	}
	selection, err := app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:     title,
		Directory: defaultDirectory,
		Filters:   filters,
	}).PromptForSingleSelection()
	if err != nil {
		return ""
	}
	return selection
}

// pickSave abre o diálogo "Salvar como". filename é o nome sugerido;
// devolve "" fora do ciclo de vida do app e no cancelamento.
func pickSave(title, defaultDirectory, filename string, filters ...application.FileFilter) string {
	app := application.Get()
	if app == nil {
		return ""
	}
	selection, err := app.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title:     title,
		Directory: defaultDirectory,
		Filename:  filename,
		Filters:   filters,
	}).PromptForSingleSelection()
	if err != nil {
		return ""
	}
	return selection
}

// WriteLog persiste um log enviado pelo frontend no MESMO sistema do
// backend: console colorido pelo nível + arquivo em JSON
// (logs/ffx-<início>.log, ao lado do executável, com source=frontend).
// Nível: debug | info | warn | error.
func (a *App) WriteLog(level, message string, fields map[string]any) error {
	return loggingService.FromFrontend(level, message, fields)
}

// PreloadVersions aquece as versões indicadas em background (uma goroutine
// worker, versões uma a uma). Chamado pelo frontend após a carga da aba
// visualizada concluir — trocar de aba cai no caminho rápido.
// versões inválidas são ignoradas com aviso no log de arquivo.
func (a *App) PreloadVersions(versions []string) {
	if a.MetadataService == nil {
		return
	}
	valid := make([]common.GameVersion, 0, len(versions))
	for _, raw := range versions {
		v, err := common.ParseGameVersionStrict(raw)
		if err != nil {
			loggingService.Warn("pré-carga: versão inválida %q", raw)
			continue
		}
		valid = append(valid, v)
	}
	a.MetadataService.PreloadVersions(valid)
}
