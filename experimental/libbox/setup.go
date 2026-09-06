package libbox

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	H "github.com/sagernet/sing-box/common/hydracore"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/locale"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/common/byteformats"
)

var (
	sBasePath                string
	sWorkingPath             string
	sTempPath                string
	sUserID                  int
	sGroupID                 int
	sFixAndroidStack         bool
	sCommandServerListenPort uint16
	sCommandServerSecret     string
	sLogMaxLines             int
	sDebug                   bool
)

func init() {
	debug.SetPanicOnFault(true)
	debug.SetTraceback("all")
}

type SetupOptions struct {
	BasePath                string
	WorkingPath             string
	TempPath                string
	FixAndroidStack         bool
	CommandServerListenPort int32
	CommandServerSecret     string
	LogMaxLines             int
	Debug                   bool
}

func Setup(options *SetupOptions) error {
	sBasePath = options.BasePath
	sWorkingPath = options.WorkingPath
	sTempPath = options.TempPath

	sUserID = os.Getuid()
	sGroupID = os.Getgid()

	// TODO: remove after fixed
	// https://github.com/golang/go/issues/68760
	sFixAndroidStack = options.FixAndroidStack

	sCommandServerListenPort = uint16(options.CommandServerListenPort)
	sCommandServerSecret = options.CommandServerSecret
	sLogMaxLines = options.LogMaxLines
	sDebug = options.Debug

	os.MkdirAll(sWorkingPath, 0o777)
	os.MkdirAll(sTempPath, 0o777)
	// The last TURN edge a transport reached outlives it in this file, so a later process
	// can offer a workerless edge probe without ever authorising into VK for an address.
	H.SetTurnEdgeStorePath(filepath.Join(sWorkingPath, "turn_edge.json"))
	return nil
}

func SetLocale(localeId string) {
	locale.Set(localeId)
}

func Version() string {
	return C.Version
}

func FormatBytes(length int64) string {
	return byteformats.FormatKBytes(uint64(length))
}

func FormatMemoryBytes(length int64) string {
	return byteformats.FormatMemoryKBytes(uint64(length))
}

func FormatDuration(duration int64) string {
	return log.FormatDuration(time.Duration(duration) * time.Millisecond)
}

func ProxyDisplayType(proxyType string) string {
	return C.ProxyDisplayName(proxyType)
}
