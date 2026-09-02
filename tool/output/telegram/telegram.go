package telegram

import (
	i18nLoc "DownloadBot/i18n"
	"DownloadBot/internal/config"
	"DownloadBot/internal/users"
	"DownloadBot/internal/ytdlp"
	"DownloadBot/tool/displayUtil/gotree"
	"DownloadBot/tool/input"
	"DownloadBot/tool/monitor"
	"DownloadBot/tool/typeTrans"
	logger "DownloadBot/tool/zap"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var deleteMes tgBotApi.DeleteMessageConfig = tgBotApi.DeleteMessageConfig{
	ChannelUsername: "",
	ChatID:          0,
	MessageID:       0,
}

func dropErr(err error) {
	if err != nil {
		logger.Panic("%w", err)
	}
}

func setCommands() tgBotApi.SetMyCommandsConfig {
	return tgBotApi.NewSetMyCommands(tgBotApi.BotCommand{
		Command:     "start",
		Description: i18nLoc.LocText("tgCommandStartDes"),
	}, tgBotApi.BotCommand{
		Command:     "myid",
		Description: i18nLoc.LocText("tgCommandMyIDDes"),
	})
}

func SendTelegramAutoUpdateMessage() func(text string) {
	MessageID := 0
	myID := typeTrans.Str2Int64(config.GetTelegramUserID())
	return func(text string) {
		if MessageID == 0 {
			msg := tgBotApi.NewMessage(myID, text)
			msg.ParseMode = "Markdown"
			res, err := tBot.Send(msg)
			dropErr(err)
			MessageID = res.MessageID
		} else {
			if text != "close" {
				newMsg := tgBotApi.NewEditMessageText(myID, MessageID, text)
				newMsg.ParseMode = "Markdown"
				tBot.Send(newMsg)
			} else {
				tBot.Send(tgBotApi.NewDeleteMessage(myID, MessageID))
			}
		}
		return
	}
}

func SendTelegramSuddenMessage(text string) {
	myID := typeTrans.Str2Int64(config.GetTelegramUserID())
	msg := tgBotApi.NewMessage(myID, text)
	msg.ParseMode = "Markdown"
	tBot.Send(msg)
}

var tBot *tgBotApi.BotAPI

// activeBot is the running bot instance used by the organize pipeline
var activeBot *tgBotApi.BotAPI

// userStore persists approved/pending/denied bot users
var userStore *users.Store

// taskStore maps task ids to the user who added them
var taskStore *users.TaskStore

// adminIDs are the configured admin chat ids (config user-id, comma separated)
var adminIDs []int64

func initUsers() {
	for _, part := range strings.Split(config.GetTelegramUserID(), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		adminIDs = append(adminIDs, typeTrans.Str2Int64(part))
	}
	userStore = users.Open("users.json")
	taskStore = users.NewTaskStore("tasks.json")
	// ensure every configured admin exists as a user
	for _, id := range adminIDs {
		userStore.SetRole(id, users.RoleAdmin)
	}
}

func isAdminID(id int64) bool {
	for _, a := range adminIDs {
		if a == id {
			return true
		}
	}
	return false
}

// notifyAdmin sends a message to the first configured admin chat.
func notifyAdmin(text string) {
	if len(adminIDs) == 0 || tBot == nil {
		return
	}
	sendPlain(tBot, adminIDs[0], text)
}

func createKeyBoardRow(texts ...string) [][]tgBotApi.KeyboardButton {
	Keyboards := make([][]tgBotApi.KeyboardButton, 0)
	for _, text := range texts {
		Keyboards = append(Keyboards, tgBotApi.NewKeyboardButtonRow(
			tgBotApi.NewKeyboardButton(text),
		))
	}
	return Keyboards
}
func createFilesInlineKeyBoardRow(filesInfos ...filesInlineKeyboards) ([][]tgBotApi.InlineKeyboardButton, string) {
	Keyboards := make([][]tgBotApi.InlineKeyboardButton, 0)
	text := ""
	index := 1
	inlineKeyBoardRow := make([]tgBotApi.InlineKeyboardButton, 0)
	for _, filesInfo := range filesInfos {
		for _, GidAndName := range filesInfo.GidAndName {

			text += fmt.Sprintf("%d: `%s`\n", index, GidAndName["Name"])
			inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(fmt.Sprint(index), GidAndName["GID"]+":"+filesInfo.Data))
			if index%7 == 0 {
				Keyboards = append(Keyboards, inlineKeyBoardRow)
				inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
			}
			index++
		}
	}
	if len(inlineKeyBoardRow) != 0 {
		Keyboards = append(Keyboards, inlineKeyBoardRow)
	}
	if text == "" {
		text = " "
	}
	return Keyboards, text[:len(text)-1]
}

func createFunctionInlineKeyBoardRow(functionInfos ...functionInlineKeyboards) []tgBotApi.InlineKeyboardButton {
	Keyboards := make([]tgBotApi.InlineKeyboardButton, 0)
	for _, functionInfo := range functionInfos {
		Keyboards = append(Keyboards, tgBotApi.NewInlineKeyboardButtonData(functionInfo.Describe, "ALL:"+functionInfo.Describe))
	}
	return Keyboards
}

func Aria2Bot(BotKey string, wg *sync.WaitGroup) {
	Keyboards := make([][]tgBotApi.KeyboardButton, 0)
	Keyboards = append(Keyboards, tgBotApi.NewKeyboardButtonRow(
		tgBotApi.NewKeyboardButton(i18nLoc.LocText("nowDownload")),
		tgBotApi.NewKeyboardButton(i18nLoc.LocText("nowWaiting")),
		tgBotApi.NewKeyboardButton(i18nLoc.LocText("nowOver")),
	))
	Keyboards = append(Keyboards, tgBotApi.NewKeyboardButtonRow(
		tgBotApi.NewKeyboardButton(i18nLoc.LocText("pauseTask")),
		tgBotApi.NewKeyboardButton(i18nLoc.LocText("resumeTask")),
		tgBotApi.NewKeyboardButton(i18nLoc.LocText("removeTask")),
	))

	Keyboards = append(Keyboards, tgBotApi.NewKeyboardButtonRow(
		tgBotApi.NewKeyboardButton(i18nLoc.LocText("removeDownloadFolderFiles")),
	))
	Keyboards = append(Keyboards, tgBotApi.NewKeyboardButtonRow(
		tgBotApi.NewKeyboardButton(i18nLoc.LocText("moveDownloadFolderFiles")),
	))

	var numericKeyboard = tgBotApi.NewReplyKeyboard(Keyboards...)

	bot, err := tgBotApi.NewBotAPI(BotKey)
	dropErr(err)
	tBot = bot
	activeBot = bot
	initUsers()
	bot.Debug = false
	// sweep crashed-run staging dirs before aria2 can deliver new events
	recoverCleanupStages()
	// delete progress messages left over from a crashed run
	inflightCleanup(bot)
	input.ToolApp.Aria2.Load(Notifier{}, func(gid string) {
		TMSelectMessageChan <- gid
	}, false)
	// aria2 is connected: resume downloads completed but never organized
	recoverResumeDownloads()

	logger.Info(fmt.Sprintf(i18nLoc.LocText("authorizedAccount"), bot.Self.UserName))
	defer wg.Done()
	// go receiveMessage(msgChan)
	go SuddenMessage(bot)
	go Aria2TMSelectMsg(bot)
	u := tgBotApi.NewUpdate(0)
	u.Timeout = 60
	_, err = bot.Request(setCommands())
	//setCommands(bot)
	updates := bot.GetUpdatesChan(u)
	dropErr(err)
	for update := range updates {
		if update.CallbackQuery != nil {
			task := strings.Split(update.CallbackQuery.Data, ":")
			//log.Println(task)
			switch task[1] {
			case "1":
				input.PauseTask(task[0])
				bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, i18nLoc.LocText("taskNowStop")))
			case "2":
				input.UnpauseTask(task[0])
				bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, i18nLoc.LocText("taskNowResume")))
			case "3":
				input.ForceRemoveTask(task[0])
				bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, i18nLoc.LocText("taskNowRemove")))
			case "4":
				input.PauseAllTask()
				bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, i18nLoc.LocText("taskNowStopAll")))
			case "5":
				input.UnpauseAllTask()
				bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, i18nLoc.LocText("taskNowResumeAll")))
			case "6":
				TMSelectMessageChan <- task[0]
				b := strings.Split(task[0], "~")
				bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, i18nLoc.LocText("selected")+b[1]))
			case "7":
				TMSelectMessageChan <- task[0]
				bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, i18nLoc.LocText("operationSuccess")))
			case "8":
				FileControlChan <- task[0]
				b := strings.Split(task[0], "~")
				bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, i18nLoc.LocText("selected")+b[1]))
			case "9":
				FileControlChan <- task[0]
				bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, i18nLoc.LocText("operationSuccess")))
			case "20":
				// approve user (admin only)
				if isAdminID(update.CallbackQuery.From.ID) {
					if parts := strings.Split(task[0], "~"); len(parts) == 2 {
						id := typeTrans.Str2Int64(parts[1])
						userStore.SetRole(id, users.RoleApproved)
						bot.Send(tgBotApi.NewMessage(id, "✅ Your access has been approved!\nSend /start to begin."))
						bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, "User approved"))
					}
				}
			case "21":
				// deny user (admin only)
				if isAdminID(update.CallbackQuery.From.ID) {
					if parts := strings.Split(task[0], "~"); len(parts) == 2 {
						id := typeTrans.Str2Int64(parts[1])
						userStore.SetRole(id, users.RoleDenied)
						bot.Send(tgBotApi.NewMessage(id, "⛔ Your access request was denied."))
						bot.Request(tgBotApi.NewCallback(update.CallbackQuery.ID, "User denied"))
					}
				}
			}

			//fmt.Print(update)

			//bot.Send(tgBotApi.NewMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Data))
		}

		if update.Message != nil { //
			msg := tgBotApi.NewMessage(update.Message.Chat.ID, "")
			msg.ParseMode = "Markdown"
			senderID := update.Message.From.ID
			senderName := strings.TrimSpace(update.Message.From.FirstName + " " + update.Message.From.LastName)
			senderUsername := update.Message.From.UserName

			// /start approval flow: register user, notify admin on new requests
			if update.Message.Command() == "start" {
				role := userStore.UpsertStarted(senderID, senderUsername, senderName)
				if isAdminID(senderID) {
					// admins fall through to normal handling below
				} else if role == users.RolePending && !userStore.Approved(senderID) {
					name := senderName
					if senderUsername != "" {
						name = "@" + senderUsername
					}
					reqMsg := tgBotApi.NewMessage(adminIDs[0], fmt.Sprintf(
						"👤 New user request\n\nID: `%d`\nName: %s\nUsername: %s\n\nApprove this user?",
						senderID, name, senderUsername))
					reqMsg.ReplyMarkup = tgBotApi.NewInlineKeyboardMarkup(
						tgBotApi.NewInlineKeyboardRow(
							tgBotApi.NewInlineKeyboardButtonData("✅ Approve", fmt.Sprintf("approve~%d:20", senderID)),
							tgBotApi.NewInlineKeyboardButtonData("⛔ Deny", fmt.Sprintf("deny~%d:21", senderID)),
						),
					)
					bot.Send(reqMsg)
					msg.Text = "👋 Welcome! Your access request has been sent to the admin.\nYou will be notified when approved."
					_, err := bot.Send(msg)
					dropErr(err)
					continue
				} else if role == users.RoleDenied {
					msg.Text = "⛔ Your access request was denied."
					_, err := bot.Send(msg)
					dropErr(err)
					continue
				}
			}

			if userStore.Approved(senderID) || isAdminID(senderID) {
				// regular users only see their own tasks; admins see all
				var allowGids map[string]bool
				if !isAdminID(senderID) {
					allowGids = map[string]bool{}
					for _, t := range taskStore.ByUser(senderID) {
						allowGids[t.GID] = true
					}
				}

				switch update.Message.Text {
				case i18nLoc.LocText("nowDownload"):
					ticker := time.NewTicker(500 * time.Millisecond)
					rand.Seed(time.Now().UnixNano())
					a := rand.Intn(100000)
					activeRefreshControl = a
					go activeRefresh(update.Message.MessageID, bot, ticker, a)
				case i18nLoc.LocText("nowWaiting"):
					res := input.ToolApp.Aria2.FormatTellWaitingFiltered(allowGids)
					if res != "" {
						msg.Text = res
					} else {
						msg.Text = i18nLoc.LocText("noWaitingTask")
					}
				case i18nLoc.LocText("nowOver"):
					res := input.ToolApp.Aria2.FormatTellStoppedFiltered(allowGids)
					if res != "" {
						msg.Text = res
					} else {
						msg.Text = i18nLoc.LocText("noOverTask")
					}
				case i18nLoc.LocText("pauseTask"):
					InlineKeyboards, text := createFilesInlineKeyBoardRow(filesInlineKeyboards{
						GidAndName: input.ToolApp.Aria2.FormatGidAndNameFiltered(0, allowGids),
						Data:       "1",
					})
					if len(InlineKeyboards) != 0 {
						msg.Text = i18nLoc.LocText("stopWhichOne") + "\n" + text
						if len(InlineKeyboards) > 1 {
							InlineKeyboards = append(InlineKeyboards, createFunctionInlineKeyBoardRow(functionInlineKeyboards{
								Describe: i18nLoc.LocText("StopAll"),
								Data:     "4",
							}))
						}
						msg.ReplyMarkup = tgBotApi.NewInlineKeyboardMarkup(InlineKeyboards...)
					} else {
						msg.Text = i18nLoc.LocText("noWaitingTask")
					}
				case i18nLoc.LocText("resumeTask"):

					InlineKeyboards, text := createFilesInlineKeyBoardRow(filesInlineKeyboards{
						GidAndName: input.ToolApp.Aria2.FormatGidAndNameFiltered(1, allowGids),
						Data:       "2",
					})
					if len(InlineKeyboards) != 0 {
						msg.Text = i18nLoc.LocText("resumeWhichOne") + "\n" + text
						if len(InlineKeyboards) > 1 {
							InlineKeyboards = append(InlineKeyboards, createFunctionInlineKeyBoardRow(functionInlineKeyboards{
								Describe: i18nLoc.LocText("ResumeAll"),
								Data:     "5",
							}))
						}
						msg.ReplyMarkup = tgBotApi.NewInlineKeyboardMarkup(InlineKeyboards...)
					} else {
						msg.Text = i18nLoc.LocText("noActiveTask")
					}
				case i18nLoc.LocText("removeTask"):

					InlineKeyboards, text := createFilesInlineKeyBoardRow(filesInlineKeyboards{
						GidAndName: input.ToolApp.Aria2.FormatGidAndNameFiltered(0, allowGids),
						Data:       "3",
					}, filesInlineKeyboards{
						GidAndName: input.ToolApp.Aria2.FormatGidAndNameFiltered(1, allowGids),
						Data:       "3",
					})
					if len(InlineKeyboards) != 0 {
						msg.Text = i18nLoc.LocText("removeWhichOne") + "\n" + text
						msg.ReplyMarkup = tgBotApi.NewInlineKeyboardMarkup(InlineKeyboards...)
					} else {
						msg.Text = i18nLoc.LocText("noOverTask")
					}
				case i18nLoc.LocText("removeDownloadFolderFiles"):
					//dropErr(removeContents(info.DownloadFolder))
					isFileChanClean := false
					for !isFileChanClean {
						select {
						case _ = <-FileControlChan:
						default:
							isFileChanClean = true
						}
					}
					FileControlChan <- "close"
					go removeFilesPrint(update.Message.MessageID, bot)
					FileControlChan <- "file"
				case i18nLoc.LocText("moveDownloadFolderFiles"):
					isFileChanClean := false
					for !isFileChanClean {
						select {
						case _ = <-FileControlChan:
						default:
							isFileChanClean = true
						}
					}
					FileControlChan <- "close"
					go copyFilesPrint(update.Message.MessageID, bot)
					FileControlChan <- "file"
				default:
					text := update.Message.Text
					switch {
					case ytdlp.IsKnownSite(text):
						// yt-dlp compatible site -> dedicated handler
						taskStore.Add(users.Task{
							GID:    "yt" + fmt.Sprint(time.Now().UnixNano()),
							UserID: senderID,
							Link:   text,
							Engine: "ytdlp",
							Status: "downloading",
						})
						notifyUserAdded(senderID, senderUsername, text)
						go startYtdlpDownload(bot, update.Message.Chat.ID, text)
					case isDownloadable(text):
						gid, ok := input.ToolApp.Aria2.Download(text)
						if ok {
							taskStore.Add(users.Task{
								GID:    gid,
								UserID: senderID,
								Link:   text,
								Engine: "aria2",
								Status: "downloading",
							})
							notifyUserAdded(senderID, senderUsername, text)
						} else {
							msg.Text = i18nLoc.LocText("unknownLink")
						}
					default:
						msg.Text = i18nLoc.LocText("unknownLink")
					}
					if update.Message.Document != nil {
						bt, _ := bot.GetFileDirectURL(update.Message.Document.FileID)
						resp, err := http.Get(bt)
						dropErr(err)
						defer resp.Body.Close()
						out, err := os.Create("temp.torrent")
						dropErr(err)
						defer out.Close()
						_, err = io.Copy(out, resp.Body)
						dropErr(err)
						if gid, ok := input.ToolApp.Aria2.Download("temp.torrent"); ok {
							_ = gid
							msg.Text = ""
						}
					}
					/*if update.Message.Video != nil {
						videoURL, _ := bot.GetFileDirectURL(update.Message.Video.FileID)
						log.Println(videoURL)
						videoInfo, err := bot.GetFile(tgBotApi.FileConfig{
							FileID: update.Message.Video.FileID,
						})
						dropErr(err)
						log.Println(videoInfo)
					}
					*/
				}

				// extract the command from the message
				switch update.Message.Command() {
				case "start":
					msg.Text = fmt.Sprintf(i18nLoc.LocText("commandStartRes"), input.ToolApp.Aria2.GetVersion())
					if monitor.IsLocal(config.GetAria2Server()) {
						msg.Text += "\n" + i18nLoc.LocText("inLocal")
					}
					//msg.Text += "\n" + locText("nowTMMode") + locText("tmMode"+aria2Set.TMMode)
					msg.ReplyMarkup = numericKeyboard
				case "help":
					msg.Text = i18nLoc.LocText("commandHelpRes")
				case "myid":
					msg.Text = fmt.Sprintf(i18nLoc.LocText("commandMyIDRes"), update.Message.Chat.ID)
				case "setMaxLength":
					i, err := strconv.Atoi(strings.ReplaceAll(update.Message.Text, "/setMaxLength ", ""))
					if err != nil {
						msg.Text = i18nLoc.LocText("commandSetMaxLengthHelpRes")
					} else {
						goTree.SetMaxLength(i)
						msg.Text = i18nLoc.LocText("operationSuccess")
					}
				}
			} else {
				msg.Text = i18nLoc.LocText("doNotHavePermissionControl")
				if update.Message.Command() == "myid" {
					msg.Text = fmt.Sprintf(i18nLoc.LocText("commandMyIDRes"), update.Message.Chat.ID)
				}
			}

			if msg.Text != "" {
				//bot.Send(tgBotApi.NewEditMessageText(update.Message.Chat.ID, 591, "123456"))
				_, err := bot.Send(msg)
				dropErr(err)
			}
		}
	}
	input.ToolApp.Aria2.Close()
}

// isDownloadable reports whether the text is an aria2-downloadable link.
func isDownloadable(text string) bool {
	return strings.Contains(text, "http://") || strings.Contains(text, "https://") ||
		strings.Contains(text, "ftp://") || strings.HasPrefix(text, "magnet:?") ||
		strings.HasSuffix(text, ".torrent")
}

// notifyUserAdded informs admins that a user added a task (plan style).
func notifyUserAdded(senderID int64, senderUsername, link string) {
	if isAdminID(senderID) {
		return // admin's own tasks do not need notifications
	}
	name := "@" + senderUsername
	if senderUsername == "" {
		u, ok := userStore.Get(senderID)
		if !ok || u.FullName == "" {
			name = fmt.Sprintf("user %d", senderID)
		} else {
			name = u.FullName
		}
	}
	notifyAdmin(fmt.Sprintf("👤 %s added a task\n🔗 %s", name, link))
}

// notifyUserTaskDone informs the task owner that their download finished.
func notifyUserTaskDone(gid, name string) {
	task, ok := taskStore.Get(gid)
	if !ok || isAdminID(task.UserID) {
		return
	}
	taskStore.SetStatus(gid, "completed")
	if activeBot == nil {
		return
	}
	sendPlain(activeBot, task.UserID, "✅ Download completed\n\n"+name)
}
