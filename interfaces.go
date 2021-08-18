// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package log

// Logger is a simple logging interface to pass to library's.
type Logger interface {
	Tracef(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Criticalf(format string, args ...interface{})

	Trace(obj interface{})
	Debug(obj interface{})
	Info(obj interface{})
	Warn(obj interface{})
	Error(obj interface{})
	Critical(obj interface{})
}

// ExtendedLogger is the interface who can be replaced by real logger implementations,
// with extended features like level.
type ExtendedLogger interface {
	Logger

	// Level returns current logging level (if supported)
	Level() Level

	// WithLevel returns a logger with logger level set to the passed argument (if supported)
	WithLevel(Level) Logger

	// 	Fields return current fields logger has set (if supported)
	Fields() map[string]interface{}

	// WithFields returns a logger with added fields (used for structured logging, if supported)
	WithFields(fields map[string]interface{}) Logger

	// Flush signal logger to empty cache (if supported)
	Flush() error
	// Close signal logger to empty cache and not add any new entry's (if supported)
	Close() error
}
