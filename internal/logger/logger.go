package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents log level
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger handles debug logging
type Logger struct {
	enabled  bool
	file     *os.File
	mu       sync.Mutex
	entries  []LogEntry
	maxMem   int // max entries in memory for UI display
	onChange func()
}

// LogEntry represents a single log entry
type LogEntry struct {
	Time    time.Time
	Level   Level
	Message string
	Source  string
}

// Format returns formatted log entry
func (e LogEntry) Format() string {
	return fmt.Sprintf("%s [%s] [%s] %s",
		e.Time.Format("2006-01-02 15:04:05.000"),
		e.Level.String(),
		e.Source,
		e.Message,
	)
}

// ShortFormat returns short format for UI
func (e LogEntry) ShortFormat() string {
	return fmt.Sprintf("[%s] [%s] %s",
		e.Time.Format("15:04:05"),
		e.Level.String(),
		e.Message,
	)
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init initializes the global logger
func Init(enabled bool) error {
	var initErr error
	once.Do(func() {
		defaultLogger = &Logger{
			enabled: enabled,
			entries: make([]LogEntry, 0),
			maxMem:  500, // keep last 500 entries in memory
		}

		if enabled {
			// Create log directory
			configDir, err := os.UserConfigDir()
			if err != nil {
				configDir = os.TempDir()
			}
			logDir := filepath.Join(configDir, "portfwd")
			if err := os.MkdirAll(logDir, 0755); err != nil {
				initErr = fmt.Errorf("failed to create log directory: %w", err)
				return
			}

			// Open log file
			logPath := filepath.Join(logDir, "debug.log")
			file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				initErr = fmt.Errorf("failed to open log file: %w", err)
				return
			}
			defaultLogger.file = file

			// Write startup marker
			defaultLogger.writeToFile(fmt.Sprintf("\n\n=== PortFwd Debug Session Started: %s ===\n",
				time.Now().Format("2006-01-02 15:04:05")))
		}
	})
	return initErr
}

// Close closes the logger
func Close() {
	if defaultLogger != nil && defaultLogger.file != nil {
		defaultLogger.writeToFile(fmt.Sprintf("=== PortFwd Debug Session Ended: %s ===\n",
			time.Now().Format("2006-01-02 15:04:05")))
		defaultLogger.file.Close()
	}
}

// SetOnChange sets callback for log changes (for UI updates)
func SetOnChange(fn func()) {
	if defaultLogger != nil {
		defaultLogger.mu.Lock()
		defaultLogger.onChange = fn
		defaultLogger.mu.Unlock()
	}
}

// IsEnabled returns true if debug logging is enabled
func IsEnabled() bool {
	return defaultLogger != nil && defaultLogger.enabled
}

// GetEntries returns recent log entries for UI display
func GetEntries() []LogEntry {
	if defaultLogger == nil {
		return nil
	}
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	
	result := make([]LogEntry, len(defaultLogger.entries))
	copy(result, defaultLogger.entries)
	return result
}

// GetLogPath returns the path to the log file
func GetLogPath() string {
	if defaultLogger == nil || defaultLogger.file == nil {
		return ""
	}
	return defaultLogger.file.Name()
}

func (l *Logger) log(level Level, source, format string, args ...interface{}) {
	if !l.enabled {
		return
	}

	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Source:  source,
		Message: fmt.Sprintf(format, args...),
	}

	l.mu.Lock()
	// Add to memory
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxMem {
		l.entries = l.entries[len(l.entries)-l.maxMem:]
	}
	onChange := l.onChange
	l.mu.Unlock()

	// Write to file
	l.writeToFile(entry.Format() + "\n")

	// Notify UI
	if onChange != nil {
		onChange()
	}
}

func (l *Logger) writeToFile(msg string) {
	if l.file != nil {
		l.file.WriteString(msg)
	}
}

// Debug logs a debug message
func Debug(source, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.log(LevelDebug, source, format, args...)
	}
}

// Info logs an info message
func Info(source, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.log(LevelInfo, source, format, args...)
	}
}

// Warn logs a warning message
func Warn(source, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.log(LevelWarn, source, format, args...)
	}
}

// Error logs an error message
func Error(source, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.log(LevelError, source, format, args...)
	}
}

// Debugf is an alias for Debug (for convenience)
func Debugf(source, format string, args ...interface{}) {
	Debug(source, format, args...)
}

// Infof is an alias for Info
func Infof(source, format string, args ...interface{}) {
	Info(source, format, args...)
}

// Warnf is an alias for Warn
func Warnf(source, format string, args ...interface{}) {
	Warn(source, format, args...)
}

// Errorf is an alias for Error
func Errorf(source, format string, args ...interface{}) {
	Error(source, format, args...)
}
