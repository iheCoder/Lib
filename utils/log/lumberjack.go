package log

import (
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	logFilePath = "default.log"
)

var (
	jarkLog = lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    100, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
		Compress:   true,
	}
)
