package config

import (
	"DownloadBot/model"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
)

var info model.Config

func dropErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func InitConfig(configPath string) {
	filePtr, err := os.Open(configPath)
	dropErr(err)
	defer filePtr.Close()
	decoder := json.NewDecoder(filePtr)
	err = decoder.Decode(&info)
	dropErr(err)
	// user-id must be exactly one numeric Telegram ID: several message
	// paths parse it as a single int64 and crash on anything else.
	id := strings.TrimSpace(info.Output.Telegram.UserID)
	if id == "" {
		log.Panic("output.telegram.user-id is empty: set it to your numeric Telegram user id")
	}
	if strings.Contains(id, ",") {
		log.Panic("output.telegram.user-id must be a single numeric Telegram ID, got: " + info.Output.Telegram.UserID)
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		log.Panic("output.telegram.user-id must be numeric, got: " + info.Output.Telegram.UserID)
	}
	info.Output.Telegram.UserID = id
}

func GetLanguage() string {
	if info.Language == "" {
		return "en"
	}
	return info.Language
}

func GetLogPath() string {
	return info.Log.LogPath
}
func GetErrPath() string {
	return info.Log.ErrPath
}
func GetLogLevel() string {
	return info.Log.Level
}

func GetDownloadFolder() string {
	return info.DownloadFolder
}

// GetOrganizeConfig returns the media library configuration
func GetOrganizeConfig() model.OrganizeConfig {
	return info.Library
}

func GetAria2Server() string {
	return info.Input.Aria2.Aria2Server
}
func GetAria2Key() string {
	return info.Input.Aria2.Aria2Key
}

// OutputToolMethod is the type of tool used to judge the output, and 0 is telegram,-1 is unconfirmed output
func OutputToolMethod() int {
	if info.Output.Telegram.UserID != "" {
		return 0
	}
	return -1
}

// InputToolMethod is the type of tool used to judge the input, and 0 is aria2,-1 is unconfirmed input
func InputToolMethod() int {
	if info.Input.Aria2.Aria2Key != "" {
		return 0
	}
	return -1
}

func GetTelegramBotKey() string {
	return info.Output.Telegram.BotKey
}

func GetTelegramUserID() string {
	return info.Output.Telegram.UserID
}

// GetMaxIndex is the maximum number of shows
func GetMaxIndex() int {
	return info.MaxIndex
}
