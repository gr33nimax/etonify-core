package log

import (
	"context"
	"sync"
	"testing"

	"github.com/sagernet/sing-box/option"
)

type recordingPlatformWriter struct {
	access   sync.Mutex
	messages []string
}

func (w *recordingPlatformWriter) WriteMessage(_ Level, message string) {
	w.access.Lock()
	defer w.access.Unlock()
	w.messages = append(w.messages, message)
}
func (w *recordingPlatformWriter) count() int {
	w.access.Lock()
	defer w.access.Unlock()
	return len(w.messages)
}

func TestDisabledFactoryEnablesExistingLogger(t *testing.T) {
	writer := new(recordingPlatformWriter)
	factory, err := New(Options{Context: context.Background(), Options: option.LogOptions{Disabled: true}, PlatformWriter: writer})
	if err != nil {
		t.Fatal(err)
	}
	logger := factory.NewLogger("test")
	logger.Error("before")
	if writer.count() != 0 {
		t.Fatal("disabled factory wrote a line")
	}
	if err = factory.(*disabledFactory).Enable(LevelError); err != nil {
		t.Fatal(err)
	}
	logger.Error("after")
	if writer.count() != 1 {
		t.Fatal("existing logger did not use the enabled factory")
	}
	factory.SetLevel(LevelPanic)
	logger.Error("off again")
	if writer.count() != 1 {
		t.Fatal("quiet level wrote after logging was turned off")
	}
}
