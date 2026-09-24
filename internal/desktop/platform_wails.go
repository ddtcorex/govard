//go:build desktop

package desktop

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// wailsPlatform adapts the Wails v2 runtime. Wails v2 only hands out its
// runtime context in OnStartup, so every call made before attachContext is a
// silent no-op (Emit) or a "desktop runtime not available" error (everything
// that returns an error).
type wailsPlatform struct {
	mu  sync.RWMutex
	ctx context.Context
}

func newDefaultPlatform() Platform { return &wailsPlatform{} }

func (p *wailsPlatform) attachContext(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ctx = ctx
}

func (p *wailsPlatform) runtimeCtx() context.Context {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ctx
}

func (p *wailsPlatform) Emit(event string, data any) {
	if ctx := p.runtimeCtx(); ctx != nil {
		runtime.EventsEmit(ctx, event, data)
	}
}

func (p *wailsPlatform) OpenURL(url string) error {
	ctx := p.runtimeCtx()
	if ctx == nil {
		return errDesktopNotAvailableFn()
	}
	runtime.BrowserOpenURL(ctx, url)
	return nil
}

func (p *wailsPlatform) ChooseDirectory(title, defaultDir string) (string, error) {
	ctx := p.runtimeCtx()
	if ctx == nil {
		return "", errDesktopNotAvailableFn()
	}
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: defaultDir,
	})
}

func (p *wailsPlatform) ChooseSaveFile(opts SaveFileOptions) (string, error) {
	ctx := p.runtimeCtx()
	if ctx == nil {
		return "", errDesktopNotAvailableFn()
	}
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:                opts.Title,
		DefaultDirectory:     opts.DefaultDir,
		DefaultFilename:      opts.DefaultFilename,
		CanCreateDirectories: true,
		Filters: []runtime.FileFilter{
			{DisplayName: "Log Files (*.log)", Pattern: "*.log"},
			{DisplayName: "Text Files (*.txt)", Pattern: "*.txt"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
}

func (p *wailsPlatform) ShowWindow() {
	if ctx := p.runtimeCtx(); ctx != nil {
		runtime.Show(ctx)
		runtime.WindowShow(ctx)
		runtime.WindowUnminimise(ctx)
	}
}

func (p *wailsPlatform) HideWindow() {
	if ctx := p.runtimeCtx(); ctx != nil {
		runtime.WindowHide(ctx)
		runtime.Hide(ctx)
	}
}

func (p *wailsPlatform) Quit() {
	if ctx := p.runtimeCtx(); ctx != nil {
		runtime.Quit(ctx)
	}
}
