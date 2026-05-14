package logger

import (
	"fmt"
	"sync"
	"time"
)

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelError Level = "error"
)

type Entry struct {
	Time    time.Time `json:"time"`
	Level   Level     `json:"level"`
	Message string    `json:"message"`
}

var (
	mu           sync.RWMutex
	entries      []Entry
	maxEntries   = 500
	currentLevel = LevelInfo
)

func SetLevel(level string) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = Level(level)
}

func GetLevel() string {
	mu.RLock()
	defer mu.RUnlock()
	return string(currentLevel)
}

func shouldLog(level Level) bool {
	mu.RLock()
	defer mu.RUnlock()

	levelPriority := map[Level]int{
		LevelDebug: 0,
		LevelInfo:  1,
		LevelError: 2,
	}

	return levelPriority[level] >= levelPriority[currentLevel]
}

func log(level Level, format string, args ...interface{}) {
	if !shouldLog(level) {
		return
	}

	entry := Entry{
		Time:    time.Now(),
		Level:   level,
		Message: fmt.Sprintf(format, args...),
	}

	mu.Lock()
	entries = append(entries, entry)
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}
	mu.Unlock()

	// 同时输出到控制台
	fmt.Printf("[%s][%s] %s\n", entry.Time.Format("15:04:05.000"), level, entry.Message)
}

func Debug(format string, args ...interface{}) {
	log(LevelDebug, format, args...)
}

func Info(format string, args ...interface{}) {
	log(LevelInfo, format, args...)
}

func Error(format string, args ...interface{}) {
	log(LevelError, format, args...)
}

func GetLogs(limit int) []Entry {
	mu.RLock()
	defer mu.RUnlock()

	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	start := len(entries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]Entry, limit)
	copy(result, entries[start:])
	return result
}

func Clear() {
	mu.Lock()
	defer mu.Unlock()
	entries = []Entry{}
}
