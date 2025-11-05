// Package logger provides a simple logging interface with configurable log levels.package logger

package logger

import (
	"io"
	"log"
	"os"
)

// Level represents the logging level
type Level int

const (
	// LevelError only shows error messages
	LevelError Level = iota
	// LevelInfo shows info and error messages (default)
	LevelInfo
	// LevelDebug shows all messages including debug output
	LevelDebug
)

// Logger represents a logger instance
type Logger struct {
	level      Level
	infoLogger  *log.Logger
	errorLogger *log.Logger
	debugLogger *log.Logger
}

var std *Logger

func init() {
	std = New(LevelInfo, os.Stdout)
}

// New creates a new logger with the specified level
func New(level Level, output io.Writer) *Logger {
	return &Logger{
		level:       level,
		infoLogger:  log.New(output, "", 0),
		errorLogger: log.New(output, "", 0),
		debugLogger: log.New(output, "", 0),
	}
}

// SetLevel sets the logging level
func SetLevel(level Level) {
	std.level = level
}

// SetLevelString sets the logging level from a string
func SetLevelString(levelStr string) {
	switch levelStr {
	case "debug":
		SetLevel(LevelDebug)
	case "info":
		SetLevel(LevelInfo)
	case "error":
		SetLevel(LevelError)
	default:
		SetLevel(LevelInfo)
	}
}

func Info(format string, v ...interface{}) {
	if std.level >= LevelInfo {
		std.infoLogger.Printf("INFO: "+format, v...)
	}
}

func Error(format string, v ...interface{}) {
	if std.level >= LevelError {
		std.errorLogger.Printf("ERROR: "+format, v...)
	}
}

// Debug logs a debug message
func Debug(format string, v ...interface{}) {
	if std.level >= LevelDebug {
		std.debugLogger.Printf("DEBUG: "+format, v...)
	}
}

// DebugHex logs a byte slice in hexadecimal format (only in debug mode)
func DebugHex(prefix string, data []byte) {
	if std.level >= LevelDebug {
		std.debugLogger.Printf("DEBUG: %s: % X\n", prefix, data)
	}
}

func Fatal(format string, v ...interface{}) {
	std.errorLogger.Printf("FATAL: "+format, v...)
	os.Exit(1)
}
