package main

import (
	i18nLoc "DownloadBot/i18n"
	"DownloadBot/internal/config"
	"DownloadBot/tool/output/telegram"
	logger "DownloadBot/tool/zap"
	"flag"
	"sync"
)

var configFile = flag.String("c", "./config.json", "config file path")

func init() {
	flag.Parse()
	config.InitConfig(*configFile)

	i18nLoc.LocLan(config.GetLanguage())

	logger.InitLog(config.GetLogPath(), config.GetErrPath(), config.GetLogLevel())
	logger.Info(i18nLoc.LocText("configCompleted"))
}

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	telegram.Aria2Bot(config.GetTelegramBotKey(), &wg)
	wg.Wait()
}
