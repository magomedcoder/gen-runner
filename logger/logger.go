package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

const (
	LevelDebug = iota
	LevelInfo
	LevelWarning
	LevelError
	LevelOff
)

var levelNames = map[int]string{
	LevelDebug:   "DEBUG",
	LevelInfo:    "INFO",
	LevelWarning: "WARN",
	LevelError:   "ERROR",
}

func ParseLevel(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarning
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

var stdLog = log.New(os.Stdout, "", 0)

type Logger struct {
	mu     sync.Mutex
	level  int
	prefix string
}

var Default = New(LevelInfo)

func New(minLevel int) *Logger {
	return &Logger{
		level:  minLevel,
		prefix: "[LLM Runner] ",
	}
}

func (l *Logger) SetLevel(level int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *Logger) output(level int, levelName string, format string, args ...any) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	msg := fmt.Sprintf(format, args...)
	stdLog.Println(l.prefix + levelName + " " + msg)
	l.mu.Unlock()
}

func (l *Logger) I(format string, args ...any) {
	l.output(LevelInfo, levelNames[LevelInfo], format, args...)
}

func (l *Logger) W(format string, args ...any) {
	l.output(LevelWarning, levelNames[LevelWarning], format, args...)
}

func (l *Logger) E(format string, args ...any) {
	l.output(LevelError, levelNames[LevelError], format, args...)
}

func I(format string, args ...any) {
	Default.I(format, args...)
}

func W(format string, args ...any) {
	Default.W(format, args...)
}

func E(format string, args ...any) {
	Default.E(format, args...)
}
