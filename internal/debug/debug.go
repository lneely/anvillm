package debug

import (
	"anvillm/internal/logging"
)

// Enabled controls whether debug logs are printed
var Enabled = false

// Log prints a log message only when debug mode is enabled
func Log(format string, args ...interface{}) {
	if Enabled {
		logging.Logger().Sugar().Infof(format, args...)
	}
}
