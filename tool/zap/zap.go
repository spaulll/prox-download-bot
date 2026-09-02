package logger

import (
	rotateLogs "github.com/lestrrat-go/file-rotatelogs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io"
	"log"
	"os"
	"time"
)

var logger *zap.Logger

// InitLog initializes the logger
func InitLog(logPath, errPath string, level string) {
	config := zapcore.EncoderConfig{
		MessageKey:  "msg",
		LevelKey:    "level",
		EncodeLevel: zapcore.CapitalLevelEncoder, // capitalize level
		TimeKey:     "ts",
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format("2006-01-02 15:04:05"))
		},
		CallerKey:    "file",
		EncodeCaller: zapcore.ShortCallerEncoder,
		EncodeDuration: func(d time.Duration, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendInt64(int64(d) / 1000000)
		},
	}
	encoder := zapcore.NewConsoleEncoder(config)
	logLevel := zap.DebugLevel
	switch level {
	case "debug":
		logLevel = zap.DebugLevel
	case "info":
		logLevel = zap.InfoLevel
	case "warn":
		logLevel = zap.WarnLevel
	case "error":
		logLevel = zap.ErrorLevel
	case "panic":
		logLevel = zap.PanicLevel
	case "fatal":
		logLevel = zap.FatalLevel
	default:
		logLevel = zap.InfoLevel
	}
	infoLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl < zapcore.WarnLevel && lvl >= logLevel
	})

	warnLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.WarnLevel && lvl >= logLevel
	})


	var zapCores []zapcore.Core
	var infoWriter, warnWriter io.Writer
	var err error
	// write info and below to logPath, warn and above to errPath
	if logPath != "" {
		infoWriter, err = getWriter(logPath)
		zapCores = append(zapCores, zapcore.NewCore(encoder, zapcore.AddSync(infoWriter), infoLevel))
	}
	if errPath != "" {
		warnWriter, err = getWriter(errPath)
		zapCores = append(zapCores, zapcore.NewCore(encoder, zapcore.AddSync(warnWriter), warnLevel))
	}
	if err != nil {
		log.Println("logging system startup exception")
		panic(err)
	}
	// all logs are also shown in the console
	zapCores = append(zapCores, zapcore.NewCore(zapcore.NewConsoleEncoder(config),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)), logLevel))

	core := zapcore.NewTee(zapCores...)
	logger = zap.New(core)
}

func getWriter(filename string) (io.Writer, error) {
	// create a rotating writer; demo.log.YYmmddHH files, demo.log links to the latest
	hook, err := rotateLogs.New(
		filename+".%Y%m%d%H",
		rotateLogs.WithLinkName(filename),
		rotateLogs.WithMaxAge(time.Hour*24*30),    // keep 30 days
		rotateLogs.WithRotationTime(time.Hour*24), // rotate daily
	)

	return hook, err
}

// logs.Debug(...)
func Debug(format string, v ...interface{}) {
	logger.Sugar().Debugf(format, v...)
}

func Info(format string, v ...interface{}) {
	logger.Sugar().Infof(format, v...)
}

func Warn(format string, v ...interface{}) {
	logger.Sugar().Warnf(format, v...)
}

func Error(format string, v ...interface{}) {
	logger.Sugar().Errorf(format, v...)
}

func Panic(format string, v ...interface{}) {
	logger.Sugar().Panicf(format, v...)
}

func DropErr(err error) {
	if err != nil {
		Panic("%w", err)
	}
}
