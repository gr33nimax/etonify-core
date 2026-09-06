package log

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing/common/observable"
)

// ErrClosed is returned to a caller that tries to use a factory the core has already closed.
var ErrClosed = errors.New("log factory is closed")

// disabledFactory keeps the logger references created during startup usable if logging is
// enabled later, and keeps the disabled path free of locks and allocations.
//
// The active factory travels in one immutable state, swapped atomically: a disabled call is
// one atomic load, an enabled call is a delegate resolved once per state and reused, so a
// suppressed line costs what an ordinary logger's line costs instead of a fresh logger per
// call on top.
type disabledFactory struct {
	access  sync.Mutex
	options Options
	active  atomic.Pointer[disabledFactoryState]
	started bool
	closed  bool
	counter uint64
}

type disabledFactoryState struct {
	factory Factory
	// Distinguishes states, so a logger's cached delegate is only trusted while the state
	// it was resolved under is still the current one.
	revision uint64
}

func newDisabledFactory(options Options) *disabledFactory { return &disabledFactory{options: options} }

func (f *disabledFactory) Start() error {
	f.access.Lock()
	defer f.access.Unlock()
	if f.closed {
		return ErrClosed
	}
	f.started = true
	if state := f.active.Load(); state != nil {
		return state.factory.Start()
	}
	return nil
}

func (f *disabledFactory) Close() error {
	f.access.Lock()
	defer f.access.Unlock()
	if f.closed {
		return nil
	}
	// Terminal: a late command must not rebuild a factory nobody will ever close again.
	f.closed = true
	state := f.active.Swap(nil)
	if state == nil {
		return nil
	}
	return state.factory.Close()
}

func (f *disabledFactory) Level() Level {
	if state := f.active.Load(); state != nil {
		return state.factory.Level()
	}
	return LevelPanic
}

func (f *disabledFactory) SetLevel(level Level) {
	if state := f.active.Load(); state != nil {
		state.factory.SetLevel(level)
	}
}

func (f *disabledFactory) Enable(level Level) error {
	f.access.Lock()
	defer f.access.Unlock()
	if f.closed {
		return ErrClosed
	}
	if state := f.active.Load(); state != nil {
		state.factory.SetLevel(level)
		return nil
	}
	options := f.options
	options.Options.Disabled = false
	active, err := New(options)
	if err != nil {
		return err
	}
	if f.started {
		if err = active.Start(); err != nil {
			// The failed factory holds whatever it did start; it is not left for the
			// process to reclaim whenever it feels like it.
			_ = active.Close()
			return err
		}
	}
	f.counter++
	f.active.Store(&disabledFactoryState{factory: active, revision: f.counter})
	active.SetLevel(level)
	return nil
}

// Disable returns the factory to its disabled fast path, releasing the active one. OFF is
// not the quietest level: it is the one setting that removes the cost of formatting lines
// nobody reads, and turning it back on has to restore that, not merely lower a threshold.
func (f *disabledFactory) Disable() error {
	f.access.Lock()
	defer f.access.Unlock()
	if f.closed {
		return ErrClosed
	}
	state := f.active.Swap(nil)
	if state == nil {
		return nil
	}
	return state.factory.Close()
}

func (f *disabledFactory) Logger() ContextLogger { return f.NewLogger("") }

func (f *disabledFactory) NewLogger(tag string) ContextLogger {
	return &disabledLogger{factory: f, tag: tag}
}

func (f *disabledFactory) Subscribe() (observable.Subscription[Entry], <-chan struct{}, error) {
	state := f.active.Load()
	if state == nil {
		return nil, nil, os.ErrInvalid
	}
	active, ok := state.factory.(ObservableFactory)
	if !ok {
		return nil, nil, os.ErrInvalid
	}
	return active.Subscribe()
}

func (f *disabledFactory) UnSubscribe(subscription observable.Subscription[Entry]) {
	state := f.active.Load()
	if state == nil {
		return
	}
	if active, ok := state.factory.(ObservableFactory); ok {
		active.UnSubscribe(subscription)
	}
}

// ContextLogger is an interface, and an atomic pointer needs a concrete type to point at.
type ContextLoggerValue struct {
	logger ContextLogger
}

type disabledLogger struct {
	factory *disabledFactory
	tag     string
	// The delegate under the revision it was resolved for; one load on the happy path.
	delegate atomic.Pointer[ContextLoggerValue]
	seen     atomic.Uint64
}

func (l *disabledLogger) resolve() ContextLogger {
	state := l.factory.active.Load()
	if state == nil {
		return nil
	}
	if cached := l.delegate.Load(); cached != nil && l.seen.Load() == state.revision {
		return cached.logger
	}
	resolved := state.factory.NewLogger(l.tag)
	l.delegate.Store(&ContextLoggerValue{logger: resolved})
	l.seen.Store(state.revision)
	return resolved
}

func (l *disabledLogger) Trace(args ...any) {
	if x := l.resolve(); x != nil {
		x.Trace(args...)
	}
}
func (l *disabledLogger) Debug(args ...any) {
	if x := l.resolve(); x != nil {
		x.Debug(args...)
	}
}
func (l *disabledLogger) Info(args ...any) {
	if x := l.resolve(); x != nil {
		x.Info(args...)
	}
}
func (l *disabledLogger) Notice(args ...any) {
	if x := l.resolve(); x != nil {
		x.Notice(args...)
	}
}
func (l *disabledLogger) Warn(args ...any) {
	if x := l.resolve(); x != nil {
		x.Warn(args...)
	}
}
func (l *disabledLogger) Error(args ...any) {
	if x := l.resolve(); x != nil {
		x.Error(args...)
	}
}
func (l *disabledLogger) Fatal(args ...any) {
	if x := l.resolve(); x != nil {
		x.Fatal(args...)
	}
}
func (l *disabledLogger) Panic(args ...any) {
	if x := l.resolve(); x != nil {
		x.Panic(args...)
	}
}
func (l *disabledLogger) TraceContext(ctx context.Context, args ...any) {
	if x := l.resolve(); x != nil {
		x.TraceContext(ctx, args...)
	}
}
func (l *disabledLogger) DebugContext(ctx context.Context, args ...any) {
	if x := l.resolve(); x != nil {
		x.DebugContext(ctx, args...)
	}
}
func (l *disabledLogger) InfoContext(ctx context.Context, args ...any) {
	if x := l.resolve(); x != nil {
		x.InfoContext(ctx, args...)
	}
}
func (l *disabledLogger) NoticeContext(ctx context.Context, args ...any) {
	if x := l.resolve(); x != nil {
		x.NoticeContext(ctx, args...)
	}
}
func (l *disabledLogger) WarnContext(ctx context.Context, args ...any) {
	if x := l.resolve(); x != nil {
		x.WarnContext(ctx, args...)
	}
}
func (l *disabledLogger) ErrorContext(ctx context.Context, args ...any) {
	if x := l.resolve(); x != nil {
		x.ErrorContext(ctx, args...)
	}
}
func (l *disabledLogger) FatalContext(ctx context.Context, args ...any) {
	if x := l.resolve(); x != nil {
		x.FatalContext(ctx, args...)
	}
}
func (l *disabledLogger) PanicContext(ctx context.Context, args ...any) {
	if x := l.resolve(); x != nil {
		x.PanicContext(ctx, args...)
	}
}
