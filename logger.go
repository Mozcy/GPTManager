package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

var appLogger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

// appDataDir 返回应用在用户 Local AppData 下的数据目录。
func appDataDir() (string, error) {
	baseDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "GPTManager"), nil
}

// initAppLogger 初始化应用日志，日志会写入控制台和本地文件。
func initAppLogger() error {
	dataDir, err := appDataDir()
	if err != nil {
		return err
	}

	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logFile, err := os.OpenFile(filepath.Join(logDir, "app.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	writer := io.MultiWriter(os.Stdout, logFile)
	appLogger = slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(appLogger)
	appLogger.Info("日志初始化完成", "path", logFile.Name())
	return nil
}
