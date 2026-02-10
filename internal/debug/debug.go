package debug

import "log"

// Enabled controls whether debug logs are printed
var Enabled = false

// Log prints a log message only when debug mode is enabled
func Log(format string, args ...interface{}) {
	if Enabled {
		log.Printf(format, args...)
	}
}
