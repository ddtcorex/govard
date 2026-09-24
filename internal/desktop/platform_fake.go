package desktop

import "sync"

// EmittedEvent is one event recorded by FakePlatform.
type EmittedEvent struct {
	Name string
	Data any
}

// DirectoryRequest is one native directory dialog request recorded by
// FakePlatform.
type DirectoryRequest struct {
	Title      string
	DefaultDir string
}

// FakePlatform records every call. It is exported for tests in ./tests.
// Configure DirectoryResult/DirectoryErr and SaveFileResult/SaveFileErr
// before use.
type FakePlatform struct {
	DirectoryResult string
	DirectoryErr    error
	SaveFileResult  string
	SaveFileErr     error
	OpenURLErr      error

	mu        sync.Mutex
	events    []EmittedEvent
	urls      []string
	dirReqs   []DirectoryRequest
	saveReqs  []SaveFileOptions
	showCount int
	hideCount int
	quitCount int
}

func (f *FakePlatform) Emit(event string, data any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, EmittedEvent{Name: event, Data: data})
}

func (f *FakePlatform) OpenURL(url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, url)
	return f.OpenURLErr
}

func (f *FakePlatform) ChooseDirectory(title string, defaultDir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dirReqs = append(f.dirReqs, DirectoryRequest{Title: title, DefaultDir: defaultDir})
	return f.DirectoryResult, f.DirectoryErr
}

func (f *FakePlatform) ChooseSaveFile(opts SaveFileOptions) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveReqs = append(f.saveReqs, opts)
	return f.SaveFileResult, f.SaveFileErr
}

func (f *FakePlatform) ShowWindow() { f.mu.Lock(); f.showCount++; f.mu.Unlock() }
func (f *FakePlatform) HideWindow() { f.mu.Lock(); f.hideCount++; f.mu.Unlock() }
func (f *FakePlatform) Quit()       { f.mu.Lock(); f.quitCount++; f.mu.Unlock() }

// Events returns a copy of every recorded event.
func (f *FakePlatform) Events() []EmittedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]EmittedEvent(nil), f.events...)
}

// EventsNamed returns the payloads of every event with the given name.
func (f *FakePlatform) EventsNamed(name string) []any {
	var out []any
	for _, e := range f.Events() {
		if e.Name == name {
			out = append(out, e.Data)
		}
	}
	return out
}

func (f *FakePlatform) OpenedURLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.urls...)
}

// DirectoryRequests returns a copy of every recorded directory dialog request.
func (f *FakePlatform) DirectoryRequests() []DirectoryRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]DirectoryRequest(nil), f.dirReqs...)
}

func (f *FakePlatform) SaveFileRequests() []SaveFileOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SaveFileOptions(nil), f.saveReqs...)
}

func (f *FakePlatform) ShowCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.showCount }
func (f *FakePlatform) HideCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.hideCount }
func (f *FakePlatform) QuitCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.quitCount }
