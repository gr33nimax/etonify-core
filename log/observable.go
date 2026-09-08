package log

import (
	"context"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing/common"
	F "github.com/sagernet/sing/common/format"
	"github.com/sagernet/sing/common/observable"
	"github.com/sagernet/sing/service/filemanager"
)

var _ Factory = (*defaultFactory)(nil)

type defaultFactory struct {
	ctx               context.Context
	formatter         Formatter
	platformFormatter Formatter
	writer            io.Writer
	file              *os.File
	filePath          string
	platformWriter    PlatformWriter
	needObservable    bool
	// Read by every logging goroutine and written by SetLevel from the command
	// thread: plain field access here was a data race, and the external mutex of
	// the factory that owns this one does not reach into Log.
	level          atomic.Uint32
	subscriber     *observable.Subscriber[Entry]
	observer       *observable.Observer[Entry]
}

func NewDefaultFactory(
	ctx context.Context,
	formatter Formatter,
	writer io.Writer,
	filePath string,
	platformWriter PlatformWriter,
	needObservable bool,
) ObservableFactory {
	factory := &defaultFactory{
		ctx:       ctx,
		formatter: formatter,
		platformFormatter: Formatter{
			BaseTime:         formatter.BaseTime,
			DisableLineBreak: true,
		},
		writer:         writer,
		filePath:       filePath,
		platformWriter: platformWriter,
		needObservable: needObservable,
		subscriber:     observable.NewSubscriber[Entry](128),
	}
	factory.level.Store(uint32(LevelTrace))
	/*if platformWriter != nil {
		factory.platformFormatter.DisableColors = platformWriter.DisableColors()
	}*/
	if needObservable {
		factory.observer = observable.NewObserver[Entry](factory.subscriber, 64)
	}
	return factory
}

func (f *defaultFactory) Start() error {
	if f.filePath != "" {
		logFile, err := filemanager.OpenFile(f.ctx, f.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		f.writer = logFile
		f.file = logFile
	}
	return nil
}

func (f *defaultFactory) Close() error {
	return common.Close(
		common.PtrOrNil(f.file),
		f.subscriber,
	)
}

func (f *defaultFactory) Level() Level {
	return Level(f.level.Load())
}

func (f *defaultFactory) SetLevel(level Level) {
	f.level.Store(uint32(level))
}

func (f *defaultFactory) Logger() ContextLogger {
	return f.NewLogger("")
}

func (f *defaultFactory) NewLogger(tag string) ContextLogger {
	return &observableLogger{f, tag}
}

func (f *defaultFactory) Subscribe() (subscription observable.Subscription[Entry], done <-chan struct{}, err error) {
	return f.observer.Subscribe()
}

func (f *defaultFactory) UnSubscribe(sub observable.Subscription[Entry]) {
	f.observer.UnSubscribe(sub)
}

var _ ContextLogger = (*observableLogger)(nil)

type observableLogger struct {
	*defaultFactory
	tag string
}

func (l *observableLogger) Log(ctx context.Context, level Level, args []any) {
	level = OverrideLevelFromContext(level, ctx)
	// One read per line: the level may change concurrently, and the branches below
	// must not each see a different one.
	enabled := l.level.Load()
	// A line below the configured level costs nothing: it is not formatted, not written, not
	// published and not handed to the platform. Both the observable subscriber and the platform
	// writer used to be exempt from this — the level gated the file writer alone.
	if level > Level(enabled) {
		return
	}
	nowTime := time.Now()
	if l.needObservable {
		message, messageSimple := l.formatter.FormatWithSimple(ctx, level, l.tag, F.ToString(args...), nowTime)
		if level == LevelPanic {
			panic(message)
		}
		l.writer.Write([]byte(message))
		if level == LevelFatal {
			os.Exit(1)
		}
		l.subscriber.Emit(Entry{level, messageSimple})
	} else {
		message := l.formatter.Format(ctx, level, l.tag, F.ToString(args...), nowTime)
		if level == LevelPanic {
			panic(message)
		}
		l.writer.Write([]byte(message))
		if level == LevelFatal {
			os.Exit(1)
		}
	}
	// The platform writer is the path that matters on Android, and it was the one still
	// unguarded. `Observable` is only true when the Clash API is on (box.go:167), so on a phone
	// the subscriber above is never even built; every core line reached the app through here
	// instead — WriteMessage into the command server, out over its log stream and across the
	// language boundary — at any level, for the platform to drop again by its own threshold.
	// Profiled at seven percent of one core with five lines a second actually being kept.
	if l.platformWriter != nil {
		l.platformWriter.WriteMessage(level, l.platformFormatter.Format(ctx, level, l.tag, F.ToString(args...), nowTime))
	}
}

func (l *observableLogger) Trace(args ...any) {
	l.TraceContext(context.Background(), args...)
}

func (l *observableLogger) Debug(args ...any) {
	l.DebugContext(context.Background(), args...)
}

func (l *observableLogger) Info(args ...any) {
	l.InfoContext(context.Background(), args...)
}

func (l *observableLogger) Notice(args ...any) {
	l.NoticeContext(context.Background(), args...)
}

func (l *observableLogger) Warn(args ...any) {
	l.WarnContext(context.Background(), args...)
}

func (l *observableLogger) Error(args ...any) {
	l.ErrorContext(context.Background(), args...)
}

func (l *observableLogger) Fatal(args ...any) {
	l.FatalContext(context.Background(), args...)
}

func (l *observableLogger) Panic(args ...any) {
	l.PanicContext(context.Background(), args...)
}

func (l *observableLogger) TraceContext(ctx context.Context, args ...any) {
	l.Log(ctx, LevelTrace, args)
}

func (l *observableLogger) DebugContext(ctx context.Context, args ...any) {
	l.Log(ctx, LevelDebug, args)
}

func (l *observableLogger) InfoContext(ctx context.Context, args ...any) {
	l.Log(ctx, LevelInfo, args)
}

func (l *observableLogger) NoticeContext(ctx context.Context, args ...any) {
	l.Log(ctx, LevelNotice, args)
}

func (l *observableLogger) WarnContext(ctx context.Context, args ...any) {
	l.Log(ctx, LevelWarn, args)
}

func (l *observableLogger) ErrorContext(ctx context.Context, args ...any) {
	l.Log(ctx, LevelError, args)
}

func (l *observableLogger) FatalContext(ctx context.Context, args ...any) {
	l.Log(ctx, LevelFatal, args)
}

func (l *observableLogger) PanicContext(ctx context.Context, args ...any) {
	l.Log(ctx, LevelPanic, args)
}
