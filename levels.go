// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package log

import (
	"fmt"
	"strings"
)

// Level is used to define severity of messages to be reported.
type Level int

const (
	// LevelUndefined is the erroneous value of log-level which corresponds
	// to zero-value.
	LevelUndefined = Level(iota)

	// LevelFatal will report about Fatalf-s only.
	LevelFatal

	// LevelCritical will report about Criticalf-s and Fatalf-s only.
	LevelCritical

	// LevelError will report about Errorf-s, Panicf-s, ...
	LevelError

	// LevelWarning will report about Warningf-s, Errorf-s, ...
	LevelWarning

	// LevelInfo will report about Infof-s, Warningf-s, ...
	LevelInfo

	// LevelDebug will report about Debugf-s, Infof-s, ...
	LevelDebug

	// LevelTrace will report about Tracef-s, Debugf-s, ...
	LevelTrace
)

// String just implements fmt.Stringer, flag.Value and pflag.Value.
func (logLevel Level) String() string {
	switch logLevel {
	case LevelUndefined:
		return "undefined"
	case LevelTrace:
		return "trace"
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarning:
		return "warning"
	case LevelError:
		return "error"
	case LevelCritical:
		return "critical"
	case LevelFatal:
		return "fatal"
	}
	return "unknown"
}

// ParseLogLevel parses incoming string into a Level and returns
// LevelUndefined with an error if an unknown logging level was passed.
func ParseLogLevel(in string) (Level, error) {
	switch strings.ToLower(in) {
	case "t", "trace":
		return LevelDebug, nil
	case "d", "debug":
		return LevelDebug, nil
	case "i", "info":
		return LevelInfo, nil
	case "w", "warn", "warning":
		return LevelWarning, nil
	case "e", "err", "error":
		return LevelError, nil
	case "c", "critical":
		return LevelCritical, nil
	case "f", "fatal":
		return LevelFatal, nil
	}
	var allowedValues []string
	for logLevel := LevelFatal; logLevel <= LevelDebug; logLevel++ {
		allowedValues = append(allowedValues, logLevel.String())
	}
	return LevelUndefined, fmt.Errorf("unknown logging level '%s', known values are: %s",
		in, strings.Join(allowedValues, ", "))
}
