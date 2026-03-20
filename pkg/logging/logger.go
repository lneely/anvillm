package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

func Init() error {
	configDir := os.Getenv("HOME")
	if configDir == "" {
		configDir = "/tmp"
	}
	logDir := filepath.Join(configDir, ".config", "anvillm", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}
	
	progName := filepath.Base(os.Args[0])
	timestamp := time.Now().Format("20060102T150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", timestamp, progName))

	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(config)

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	logLevel := zapcore.InfoLevel
	if os.Getenv("ANVILLM_DEBUG") != "" {
		logLevel = zapcore.DebugLevel
	}

	core := zapcore.NewCore(fileEncoder, zapcore.AddSync(logFile), logLevel)
	logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	logger.Info("logging initialized", zap.String("program", progName), zap.String("log_file", logPath), zap.String("level", logLevel.String()))
	return nil
}

func Logger() *zap.Logger {
	if logger == nil {
		logger = zap.NewNop()
	}
	return logger
}
