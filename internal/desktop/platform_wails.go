//go:build desktop

package desktop

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// wailsPlatform adapts Wails v3. The app and window exist only after
// application.New, which needs the services first, so they are attached late;
// calls before attach are no-ops (Emit, window control) or errors (dialogs).
type wailsPlatform struct {
	mu     sync.RWMutex
	app    *application.App
	window *application.WebviewWindow

	// gate records an explicit quit so the window close hook stops cancelling
	// closes: app.Quit() closes the window, and a cancelled close would make
	// Quit a no-op.
	gate closeGate
}

func newDefaultPlatform() Platform { return &wailsPlatform{} }

func (p *wailsPlatform) attach(app *application.App, window *application.WebviewWindow) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.app, p.window = app, window
}

func (p *wailsPlatform) handles() (*application.App, *application.WebviewWindow) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.app, p.window
}

func (p *wailsPlatform) Emit(event string, data any) {
	if app, _ := p.handles(); app != nil {
		app.Event.Emit(event, data)
	}
}

func (p *wailsPlatform) OpenURL(url string) error {
	app, _ := p.handles()
	if app == nil {
		return errDesktopNotAvailableFn()
	}
	return app.Browser.OpenURL(url)
}

func (p *wailsPlatform) ChooseDirectory(title, defaultDir string) (string, error) {
	app, _ := p.handles()
	if app == nil {
		return "", errDesktopNotAvailableFn()
	}
	return app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle(title).
		SetDirectory(defaultDir).
		PromptForSingleSelection()
}

func (p *wailsPlatform) ChooseSaveFile(opts SaveFileOptions) (string, error) {
	app, _ := p.handles()
	if app == nil {
		return "", errDesktopNotAvailableFn()
	}
	return app.Dialog.SaveFile().
		SetMessage(opts.Title).
		SetDirectory(opts.DefaultDir).
		SetFilename(opts.DefaultFilename).
		CanCreateDirectories(true).
		AddFilter("Log Files (*.log)", "*.log").
		AddFilter("Text Files (*.txt)", "*.txt").
		AddFilter("All Files (*.*)", "*.*").
		PromptForSingleSelection()
}

func (p *wailsPlatform) ShowWindow() {
	if _, w := p.handles(); w != nil {
		w.Show()
		w.UnMinimise()
		w.Focus()
	}
}

func (p *wailsPlatform) HideWindow() {
	if _, w := p.handles(); w != nil {
		w.Hide()
	}
}

func (p *wailsPlatform) Quit() {
	p.gate.MarkQuitting()
	if app, _ := p.handles(); app != nil {
		app.Quit()
	}
}
