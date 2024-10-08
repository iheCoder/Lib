package log

import (
	"context"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var (
	globalLogger *zap.Logger
)

func init() {

	// json encoder
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// new logger with core
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), zapcore.AddSync(&jarkLog)),
		zapcore.InfoLevel,
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	defer logger.Sync()

	// replace global logger
	globalLogger = logger
}

func Info(ctx context.Context, msg string, fields ...zap.Field) {
	globalLogger.Info(msg, fields...)
}

func Error(ctx context.Context, msg string, fields ...zap.Field) {
	globalLogger.Error(msg, fields...)
}

func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	globalLogger.Debug(msg, fields...)
}
