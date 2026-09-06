package log

import (
	"context"
	"os"
	"sync"

	"github.com/sagernet/sing/common/observable"
)

// disabledFactory keeps the logger references created during startup usable if logging is enabled later.
type disabledFactory struct {
	access  sync.RWMutex
	options Options
	active  Factory
	started bool
}

func newDisabledFactory(options Options) *disabledFactory { return &disabledFactory{options: options} }

func (f *disabledFactory) Start() error {
	f.access.Lock()
	f.started = true
	active := f.active
	f.access.Unlock()
	if active != nil {
		return active.Start()
	}
	return nil
}

func (f *disabledFactory) Close() error {
	f.access.Lock()
	active := f.active
	f.active = nil
	f.access.Unlock()
	if active != nil {
		return active.Close()
	}
	return nil
}

func (f *disabledFactory) Level() Level {
	f.access.RLock()
	defer f.access.RUnlock()
	if f.active == nil {
		return LevelPanic
	}
	return f.active.Level()
}

func (f *disabledFactory) SetLevel(level Level) {
	f.access.RLock()
	active := f.active
	f.access.RUnlock()
	if active != nil {
		active.SetLevel(level)
	}
}

func (f *disabledFactory) Enable(level Level) error {
	f.access.Lock()
	if f.active == nil {
		options := f.options
		options.Options.Disabled = false
		active, err := New(options)
		if err != nil {
			f.access.Unlock()
			return err
		}
		f.active = active
		if f.started {
			if err = active.Start(); err != nil {
				f.active = nil
				f.access.Unlock()
				return err
			}
		}
	}
	active := f.active
	f.access.Unlock()
	active.SetLevel(level)
	return nil
}

func (f *disabledFactory) Logger() ContextLogger { return f.NewLogger("") }

func (f *disabledFactory) NewLogger(tag string) ContextLogger {
	return &disabledLogger{factory: f, tag: tag}
}

func (f *disabledFactory) logger(tag string) ContextLogger {
	f.access.RLock()
	active := f.active
	f.access.RUnlock()
	if active == nil {
		return nil
	}
	return active.NewLogger(tag)
}

func (f *disabledFactory) Subscribe() (observable.Subscription[Entry], <-chan struct{}, error) {
	f.access.RLock()
	active, ok := f.active.(ObservableFactory)
	f.access.RUnlock()
	if !ok {
		return nil, nil, os.ErrInvalid
	}
	return active.Subscribe()
}

func (f *disabledFactory) UnSubscribe(subscription observable.Subscription[Entry]) {
	f.access.RLock()
	active, ok := f.active.(ObservableFactory)
	f.access.RUnlock()
	if ok {
		active.UnSubscribe(subscription)
	}
}

type disabledLogger struct {
	factory *disabledFactory
	tag     string
}

func (l *disabledLogger) logger() ContextLogger { return l.factory.logger(l.tag) }
func (l *disabledLogger) Trace(args ...any) {
	if x := l.logger(); x != nil {
		x.Trace(args...)
	}
}
func (l *disabledLogger) Debug(args ...any) {
	if x := l.logger(); x != nil {
		x.Debug(args...)
	}
}
func (l *disabledLogger) Info(args ...any) {
	if x := l.logger(); x != nil {
		x.Info(args...)
	}
}
func (l *disabledLogger) Notice(args ...any) {
	if x := l.logger(); x != nil {
		x.Notice(args...)
	}
}
func (l *disabledLogger) Warn(args ...any) {
	if x := l.logger(); x != nil {
		x.Warn(args...)
	}
}
func (l *disabledLogger) Error(args ...any) {
	if x := l.logger(); x != nil {
		x.Error(args...)
	}
}
func (l *disabledLogger) Fatal(args ...any) {
	if x := l.logger(); x != nil {
		x.Fatal(args...)
	}
}
func (l *disabledLogger) Panic(args ...any) {
	if x := l.logger(); x != nil {
		x.Panic(args...)
	}
}
func (l *disabledLogger) TraceContext(ctx context.Context, args ...any) {
	if x := l.logger(); x != nil {
		x.TraceContext(ctx, args...)
	}
}
func (l *disabledLogger) DebugContext(ctx context.Context, args ...any) {
	if x := l.logger(); x != nil {
		x.DebugContext(ctx, args...)
	}
}
func (l *disabledLogger) InfoContext(ctx context.Context, args ...any) {
	if x := l.logger(); x != nil {
		x.InfoContext(ctx, args...)
	}
}
func (l *disabledLogger) NoticeContext(ctx context.Context, args ...any) {
	if x := l.logger(); x != nil {
		x.NoticeContext(ctx, args...)
	}
}
func (l *disabledLogger) WarnContext(ctx context.Context, args ...any) {
	if x := l.logger(); x != nil {
		x.WarnContext(ctx, args...)
	}
}
func (l *disabledLogger) ErrorContext(ctx context.Context, args ...any) {
	if x := l.logger(); x != nil {
		x.ErrorContext(ctx, args...)
	}
}
func (l *disabledLogger) FatalContext(ctx context.Context, args ...any) {
	if x := l.logger(); x != nil {
		x.FatalContext(ctx, args...)
	}
}
func (l *disabledLogger) PanicContext(ctx context.Context, args ...any) {
	if x := l.logger(); x != nil {
		x.PanicContext(ctx, args...)
	}
}
