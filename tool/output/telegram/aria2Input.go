package telegram

import (
	i18nLoc "DownloadBot/i18n"
	"DownloadBot/tool/displayUtil/gotree"
	"DownloadBot/tool/input"
	"DownloadBot/tool/input/aria2"
	"DownloadBot/tool/input/aria2/rpc"
	"DownloadBot/tool/typeTrans"
	logger "DownloadBot/tool/zap"
	"fmt"
	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// suddenMsg carries an urgent notice plus its task gid for multi-user routing.
type suddenMsg struct {
	GID  string
	Text string
}

// SuddenMessageChan is a channel for reminding when an emergency message occurs, such as download start and download end
var SuddenMessageChan = make(chan suddenMsg, 10)

// TMSelectMessageChan is a channel for reminding when you upload a torrent file or send a magnet(TM is Torrent/Magnet)
var TMSelectMessageChan = make(chan string, 10)

// SuddenMessage delivers urgent notices (start/pause/stop/error) to BOTH the
// task owner and all admins so regular users see feedback for their own tasks.
func SuddenMessage(bot *tgBotApi.BotAPI) {
	for m := range SuddenMessageChan {
		text := m.Text
		if strings.Contains(text, "[{") && m.GID != "" {
			text = strings.Replace(text, m.GID, input.ToolApp.Aria2.TellName(m.GID), -1)
		}
		var chats []int64
		if m.GID == "" {
			chats = adminChatIDs()
		} else {
			chats = chatsForGid(m.GID)
		}
		for _, chat := range dedupChats(chats) {
			msg := tgBotApi.NewMessage(chat, text)
			res, err := bot.Send(msg)
			if err != nil {
				logger.Error("sudden message send failed: %v", err)
				continue
			}
			// remember "Download started!" notices so they can be dropped when
			// the task ends
			if strings.HasSuffix(text, startedNoticeSuffix()) && strings.Contains(text, "started") && m.GID != "" {
				rememberStartedNotice(m.GID, chat, res.MessageID)
			}
		}
	}
}

// Aria2TMSelectMsg handles torrent/magnet file selection. The picker is
// mirrored to BOTH the task owner and all admins so whoever added the task
// sees feedback and either side can confirm the selection.
func Aria2TMSelectMsg(bot *tgBotApi.BotAPI) {
	pickerMsgIDs := map[int64][]int{}
	var pickerChats []int64
	var pickerGID string
	selectFileList := make([][2]int, 0)
	directoryTree := make(map[string]interface{}, 0)

	deletePickerMsgs := func() {
		for chat, ids := range pickerMsgIDs {
			for _, id := range ids {
				bot.Send(tgBotApi.NewDeleteMessage(chat, id))
				inflightRemoveChat(chat, id)
			}
		}
		pickerMsgIDs = map[int64][]int{}
	}
	resetPicker := func() {
		pickerGID = ""
		pickerChats = nil
		selectFileList = make([][2]int, 0)
		directoryTree = make(map[string]interface{}, 0)
	}
	// sendChunk mirrors one picker page to every chat: new messages are sent,
	// existing ones edited in place. msgIdx is the zero-based page index.
	sendChunk := func(text string, keyboards [][]tgBotApi.InlineKeyboardButton, msgIdx int) {
		markup := tgBotApi.NewInlineKeyboardMarkup(keyboards...)
		for _, chat := range pickerChats {
			ids := pickerMsgIDs[chat]
			if len(ids) < msgIdx+1 {
				msg := tgBotApi.NewMessage(chat, text)
				msg.ReplyMarkup = markup
				res, err := bot.Send(msg)
				if err != nil {
					logger.Error("picker send failed: %v", err)
					continue
				}
				pickerMsgIDs[chat] = append(pickerMsgIDs[chat], res.MessageID)
				inflightAdd(chat, res.MessageID)
			} else {
				newMsg := tgBotApi.NewEditMessageTextAndMarkup(chat, ids[msgIdx], text, markup)
				if _, err := bot.Send(newMsg); err != nil {
					logger.Debug("picker edit failed: %v", err)
				}
			}
		}
	}

	for {
		a := <-TMSelectMessageChan
		b := strings.Split(a, "~")
		// 0 is gid, 1 is control sign(Start/SelectAll...) / selected num
		gid := b[0]

		downloadFilesCount := 0
		if len(b) != 1 {
			if pickerGID != "" && gid != pickerGID {
				// stale button from a previous picker - ignore
				continue
			}
			if b[1] == "Start" { //click start
				input.ToolApp.Aria2.SetTMDownloadFilesAndStart(gid, selectFileList)
				deletePickerMsgs()
				resetPicker()
				continue
			} else if b[1] == "cancel" {
				deletePickerMsgs()
				resetPicker()
				input.ToolApp.Aria2.ForceRemove(gid)
				continue
			}
			for i := 0; i < len(selectFileList); i++ {
				if selectFileList[i][0] == 1 && selectFileList[i][1] >= 0 {
					downloadFilesCount++
				}
			}
			switch b[1] {
			case "selectAll": //click select all
				for i := 0; i < len(selectFileList); i++ {
					selectFileList[i][0] = 1
				}
			case "tmMode1": //click selectBiggestFile
				biggestFileIndex := input.ToolApp.Aria2.SelectBiggestFile(gid)
				for i := 0; i < len(selectFileList); i++ {
					if selectFileList[i][1] != biggestFileIndex {
						selectFileList[i][0] = 0
					} else {
						selectFileList[i][0] = 1
						if i != 0 && selectFileList[i-1][1] == -1 {
							// if there is only one file in the folder to which this file belongs, select it as well
							selectFileList[i-1][0] = 1
						}

					}
				}
			case "tmMode2": // click smart ....
				bigFilesIndex := input.ToolApp.Aria2.SelectBigFiles(gid)
				index2Original := make(map[int]int, 0)
				for i := 0; i < len(selectFileList); i++ {
					selectFileList[i][0] = 0
					if selectFileList[i][1] > 0 { // node do not need records
						index2Original[selectFileList[i][1]] = i
					}
				}
				for _, i := range bigFilesIndex {
					if idx, ok := index2Original[i]; ok && idx >= 0 && idx < len(selectFileList) {
						selectFileList[idx][0] = 1
					}
				}
			default:
				i := typeTrans.Str2Int(b[1]) - 1 // sequence num displayed is more than the subscript,so reduce it
				if i < 0 || i >= len(selectFileList) {
					// stale button from a previous picker - ignore
					break
				}
				if downloadFilesCount > 1 { // make sure that at least one file will be downloaded
					if selectFileList[i][1] >= 0 {
						if selectFileList[i][0] == 1 {
							selectFileList[i][0] = 0
						} else {
							selectFileList[i][0] = 1
						}
					} else { //if select node
						if selectFileList[i][0] == 1 {
							selectFileList[i][0] = 0
							for s := 1; s < selectFileList[i][1]*-1 && i+s < len(selectFileList); s++ {
								selectFileList[i+s][0] = 0
								downloadFilesCount--
							}
							if downloadFilesCount < 1 {
								selectFileList[i][0] = 1
								for s := 1; s < selectFileList[i][1]*-1 && i+s < len(selectFileList); s++ {
									selectFileList[i+s][0] = 1
									downloadFilesCount++
								}
							}
						} else {
							selectFileList[i][0] = 1
							for s := 1; s < selectFileList[i][1]*-1 && i+s < len(selectFileList); s++ {
								selectFileList[i+s][0] = 1
							}
						}
					}
				}
			}

			for i, val := range selectFileList { // check weather all files under all nodes select status
				if val[1] < 0 {
					allSelected := true
					allNotSelected := true
					// log.Println(i+1,i+1+val[1]*-1-1,val[1],)
					end := i + 1 + val[1]*-1 - 1
					if end > len(selectFileList) {
						end = len(selectFileList)
					}
					if i+1 > end {
						continue
					}
					for _, val1 := range selectFileList[i+1 : end] {
						if val1[0] == 0 {
							allSelected = false
						} else if val1[0] == 1 {
							allNotSelected = false
						}
						if !allSelected && !allNotSelected {
							break
						}
					}
					if !allSelected && !allNotSelected { // partially selected, partially not be selected
						selectFileList[i][0] = -1
					} else if allSelected {
						selectFileList[i][0] = 1
					} else {
						selectFileList[i][0] = 0
					}

				}
			}
		} else {
			// new torrent/magnet: if another picker is still open, clear it
			// first so its messages do not linger
			if pickerGID != "" && pickerGID != gid {
				deletePickerMsgs()
			}
			resetPicker()
			pickerGID = gid
			pickerChats = dedupChats(chatsForGid(gid))
			if len(pickerChats) == 0 {
				pickerChats = adminChatIDs()
			}
			fileList := input.ToolApp.Aria2.FormatTMFiles(gid)
			// magnet metadata arrives asynchronously - poll briefly instead
			// of building a picker from an empty file list (panic)
			for i := 0; i < 15 && len(fileList) == 0; i++ {
				time.Sleep(time.Second)
				fileList = input.ToolApp.Aria2.FormatTMFiles(gid)
			}
			if len(fileList) == 0 {
				// metadata never arrived (dead magnet / removed task) -
				// release the pause so nothing sticks, skip the picker
				logger.Error("torrent file list empty for %s, starting as-is", gid)
				input.ToolApp.Aria2.SetTMDownloadFilesAndStart(gid, nil)
				resetPicker()
				continue
			}
			if len(fileList) == 1 {
				// single-file torrent: no selection needed, start directly
				input.ToolApp.Aria2.SetTMDownloadFilesAndStart(gid, [][2]int{{1, 1}})
				resetPicker()
				continue
			}
			index := 1
			for i, file := range fileList {
				pathClass(fmt.Sprintf("%s|%s|%d", file[0], file[1], i+1), &directoryTree)
				//logger.Info("%s|%s|%d\n", file[0], file[1], i+1)
				index++
			}
		}

		text := fmt.Sprintf("%s %s\n", input.ToolApp.Aria2.TellName(gid), i18nLoc.LocText("fileDirectoryIsAsFollows"))
		Keyboards := make([][]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow := make([]tgBotApi.InlineKeyboardButton, 0)

		fileListGoTree, _, _ := generateGoTree(directoryTree, 0, &selectFileList)
		if len(fileListGoTree) == 0 {
			// tree build produced nothing (unexpected shape) - start the
			// download as-is instead of indexing an empty slice (panic)
			logger.Error("torrent tree empty for %s, starting as-is", gid)
			input.ToolApp.Aria2.SetTMDownloadFilesAndStart(gid, nil)
			deletePickerMsgs()
			resetPicker()
			continue
		}
		fileListTreeLine := strings.Split(fileListGoTree[0].Print(), "\n")
		fileListTreeLineCount := len(fileListTreeLine)
		characterCount := len(text)
		r, err := regexp.Compile(`[✅⬜〰](\d+)`)
		dropErr(err)
		startAndEndIndex := []int{1, 0}
		msgCount := 0

		for i, line := range fileListTreeLine {
			if fileListTreeLineCount != i+1 && characterCount+len(fileListTreeLine[i+1]) > 4096 {

				msgCount++
				//lastFilesInfo = fileList
				index := 0
				for j := startAndEndIndex[0]; j <= startAndEndIndex[1]; j++ {
					inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(fmt.Sprint(j), gid+"~"+fmt.Sprint(j)+":6"))
					index++
					if index%7 == 0 {
						Keyboards = append(Keyboards, inlineKeyBoardRow)
						inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
					}

				}
				if len(inlineKeyBoardRow) != 0 {
					Keyboards = append(Keyboards, inlineKeyBoardRow)
				}
				sendChunk(text, Keyboards, msgCount-1)

				Keyboards = make([][]tgBotApi.InlineKeyboardButton, 0)
				inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)

				//lastGid = gid
				characterCount = 0
				text = "·"
				startAndEndIndex[0] = startAndEndIndex[1] + 1
			}

			if text == "·" {
				text += line[1:] + "\n"
			} else {
				text += line + "\n"
			}
			characterCount += len(line + "\n")
			res := r.FindStringSubmatch(line)
			if res != nil {
				startAndEndIndex[1] = typeTrans.Str2Int(res[1])
			}
		}

		// log.Println(text)
		text += i18nLoc.LocText("pleaseSelectTheFileYouWantToDownload")
		if msgCount > 2 {
			text += "\n" + i18nLoc.LocText("tmFileTooMany")
		}
		index := 0
		for j := startAndEndIndex[0]; j <= startAndEndIndex[1]; j++ {
			inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(fmt.Sprint(j), gid+"~"+fmt.Sprint(j)+":6"))
			index++
			if index%7 == 0 {
				Keyboards = append(Keyboards, inlineKeyBoardRow)
				inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
			}

		}

		if len(inlineKeyBoardRow) != 0 {
			Keyboards = append(Keyboards, inlineKeyBoardRow)
		}
		inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("selectAll"), gid+"~selectAll"+":7"))
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("cancel"), gid+"~cancel"+":7"))
		Keyboards = append(Keyboards, inlineKeyBoardRow)
		inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("tmMode1"), gid+"~tmMode1"+":7"))
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("tmMode2"), gid+"~tmMode2"+":7"))
		Keyboards = append(Keyboards, inlineKeyBoardRow)
		inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("startDownload"), gid+"~Start"+":7"))
		Keyboards = append(Keyboards, inlineKeyBoardRow)

		//myID, err := strconv.ParseInt(info.UserID, 10, 64)
		//dropErr(err)

		//lastFilesInfo = fileList
		sendChunk(text, Keyboards, msgCount)

	}
}

const FileMarker = "<files>"

// pathClass is a function that can generate a directory tree structure when giving a file path list. val is path, trunk is the directory structure
func pathClass(branch string, trunk *map[string]interface{}) {
	parts := strings.SplitN(branch, "/", 2)
	if len(parts) == 1 {
		if (*trunk)[FileMarker] == nil {
			fileNameList := make([]string, 0)
			fileNameList = append(fileNameList, parts[0])
			(*trunk)[FileMarker] = fileNameList
		} else {
			fileNameList := (*trunk)[FileMarker].([]string)
			fileNameList = append(fileNameList, parts[0])
			(*trunk)[FileMarker] = fileNameList
		}
	} else {
		node, other := parts[0], parts[1]
		if _, ok := (*trunk)[node]; !ok {
			(*trunk)[node] = map[string]interface{}{}
		}
		_temp := (*trunk)[node].(map[string]interface{})
		pathClass(other, &_temp)
	}
}

var (
	activeRefreshMu      sync.Mutex
	activeRefreshControl = map[int64]int{}
)

// setActiveRefreshControl replaces the auto/live view flag for one chat so
// admin and regular users can each hold an independent live view.
func setActiveRefreshControl(chatID int64, flag int) {
	activeRefreshMu.Lock()
	activeRefreshControl[chatID] = flag
	activeRefreshMu.Unlock()
}

func getActiveRefreshControl(chatID int64) int {
	activeRefreshMu.Lock()
	defer activeRefreshMu.Unlock()
	return activeRefreshControl[chatID]
}

// activeRefresh refreshes the download info in one chat, showing only the
// requester's own tasks (admins see everything).
func activeRefresh(requestChatID int64, chatMsgID int, bot *tgBotApi.BotAPI, ticker *time.Ticker, flag int) {
	var MessageID = 0

	allow := allowGidsFor(requestChatID)
	refreshPath := func(MessageID int, bot *tgBotApi.BotAPI, ticker *time.Ticker) int {
		res := input.ToolApp.Aria2.FormatTellActiveFiltered(allow)
		//log.Println(res, len(res))
		text := ""
		if res != "" {
			text = res
		} else {
			text = i18nLoc.LocText("noActiveTask")
		}
		if MessageID == 0 {
			msg := tgBotApi.NewMessage(requestChatID, text)
			msg.ParseMode = "Markdown"
			res, err := bot.Send(msg)
			if err != nil {
				logger.Error("active refresh send failed: %v", err)
				ticker.Stop()
				return -1
			}
			if text == i18nLoc.LocText("noActiveTask") {
				ticker.Stop()
				return -1
			} else {
				return res.MessageID
			}
		} else {
			if text == i18nLoc.LocText("noActiveTask") {
				bot.Send(tgBotApi.NewDeleteMessage(requestChatID, MessageID))
				ticker.Stop()
				return -1
			} else {
				newMsg := tgBotApi.NewEditMessageText(requestChatID, MessageID, text)
				newMsg.ParseMode = "Markdown"
				bot.Send(newMsg)
				return newMsg.MessageID
			}

		}
	}

	for {
		if getActiveRefreshControl(requestChatID) != flag {
			if MessageID != 0 {
				bot.Send(tgBotApi.NewDeleteMessage(requestChatID, MessageID))
			}
			ticker.Stop()
			if chatMsgID != 0 {
				msgToDelete := tgBotApi.DeleteMessageConfig{
					ChatID:    requestChatID,
					MessageID: chatMsgID,
				}
				_, _ = bot.Request(msgToDelete)
			}
			return
		} else {
			if MessageID != 0 {
				select {
				case _ = <-ticker.C:
					MessageID = refreshPath(MessageID, bot, ticker)
					if MessageID == -1 {
						return
					}
				}
			} else {
				MessageID = refreshPath(MessageID, bot, ticker)
				if MessageID == -1 {
					return
				}
			}
		}
	}
}

// startActiveRefresh replaces any previous live view in chatID with a new one.
func startActiveRefresh(chatID int64, chatMsgID int) {
	if activeBot == nil {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	rand.Seed(time.Now().UnixNano())
	flag := rand.Intn(100000) + 1
	setActiveRefreshControl(chatID, flag)
	go activeRefresh(chatID, chatMsgID, activeBot, ticker, flag)
}

// generateGoTree is a function that receive the directory tree structure generated by pathClass(),and file list that user want to select ,to generate goTree,return both goTree and the list of selected files
func generateGoTree(m map[string]interface{}, index int, selectFileList *[][2]int) ([]goTree.Tree, int, int) {
	var artist []goTree.Tree
	allFilesOwnedByNodeCount := 0
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// grow is a bounds-safe accessor for selectFileList entries
	grow := func(idx int) {
		for len(*selectFileList) <= idx {
			*selectFileList = append(*selectFileList, [2]int{1, 0})
		}
	}
	for _, k := range keys {
		// top-level files without a directory (e.g. single file whose path
		// had no "/"): pathClass stores them under FileMarker directly
		if k == FileMarker {
			ls, ok := m[k].([]string)
			if !ok {
				continue
			}
			for _, u := range ls {
				_fileInfo := strings.Split(u, "|")
				if len(_fileInfo) < 3 {
					continue
				}
				grow(index)
				_selectAndOriginalNum := [2]int{1, typeTrans.Str2Int(_fileInfo[2])}
				*selectFileList = append(*selectFileList, _selectAndOriginalNum)
				if (*selectFileList)[index][0] == 1 {
					artist = append(artist, goTree.New(fmt.Sprintf("✅%d: %s|%s", index+1, _fileInfo[0], _fileInfo[1])))
				} else {
					artist = append(artist, goTree.New(fmt.Sprintf("⬜%d: %s|%s", index+1, _fileInfo[0], _fileInfo[1])))
				}
				index++
				allFilesOwnedByNodeCount++
			}
			continue
		}
		var _artist goTree.Tree
		filesOwnedByNodeCount := 0
		switch vv := m[k].(type) {
		case map[string]interface{}:
			//fmt.Println(k, "is an map:")
			var isEmpty = false
			var nodeIndex = index
			index++
			if len(*selectFileList) < index {
				_selectAndOriginalNum := [2]int{1, 0} //node has no original num
				*selectFileList = append(*selectFileList, _selectAndOriginalNum)
				isEmpty = true
			}
			grow(index - 1)
			if (*selectFileList)[index-1][0] == 1 {
				_artist = goTree.New(fmt.Sprintf("✅%d: %s", index, k))
			} else if (*selectFileList)[index-1][0] == 0 {
				_artist = goTree.New(fmt.Sprintf("⬜%d: %s", index, k))
			} else {
				_artist = goTree.New(fmt.Sprintf("〰%d: %s", index, k))
			}

			if vs, ok := vv[FileMarker]; ok {
				ls := vs.([]string)
				//flag=true
				for _, u := range ls {
					index++
					filesOwnedByNodeCount++
					_fileInfo := strings.Split(u, "|")
					//0 is file name,1 is file size, 2 is original num
					if len(_fileInfo) < 3 {
						filesOwnedByNodeCount--
						index--
						continue
					}

					if isEmpty {
						_selectAndOriginalNum := [2]int{1, typeTrans.Str2Int(_fileInfo[2])}
						*selectFileList = append(*selectFileList, _selectAndOriginalNum)
					}

					grow(index - 1)
					if (*selectFileList)[index-1][0] == 1 {
						_artist.Add(fmt.Sprintf("✅%d: %s|%s", index, _fileInfo[0], _fileInfo[1]))
					} else {
						_artist.Add(fmt.Sprintf("⬜%d: %s|%s", index, _fileInfo[0], _fileInfo[1]))
					}
				}
			}
			res, _index, _filesOwnedByNodeCount := generateGoTree(vv, index, selectFileList)
			index = _index
			filesOwnedByNodeCount += _filesOwnedByNodeCount + 1
			if res != nil {
				for _, vk := range res {
					_artist.AddTree(vk)
				}
			}
			artist = append(artist, _artist)
			grow(nodeIndex)
			(*selectFileList)[nodeIndex][1] = filesOwnedByNodeCount * -1 // node has no original num, so we can set the second to the number of file subordinate to this node,but to prevent confusion with original num, we will take a negative
		}
		allFilesOwnedByNodeCount += filesOwnedByNodeCount
	}
	return artist, index, allFilesOwnedByNodeCount
}

type Notifier struct {
}

// startedNoticeSuffix is the localized tail of a "Download started!" notice.
// Computed lazily - the localizer is not ready during package init.
func startedNoticeSuffix() string {
	return strings.TrimPrefix(i18nLoc.LocText("onDownloadStartDes"), "%s")
}

// startedNotices maps gid -> per-chat message ids of its "Download started!"
// notices so every mirrored notice can be removed once the task ends.
var (
	startedNoticesMu sync.Mutex
	startedNotices   = map[string][]chatMsgID{}
)

func rememberStartedNotice(gid string, chatID int64, msgID int) {
	if gid == "" || msgID <= 0 || chatID == 0 {
		return
	}
	startedNoticesMu.Lock()
	for _, m := range startedNotices[gid] {
		if m.ChatID == chatID && m.MsgID == msgID {
			startedNoticesMu.Unlock()
			return
		}
	}
	startedNotices[gid] = append(startedNotices[gid], chatMsgID{ChatID: chatID, MsgID: msgID})
	startedNoticesMu.Unlock()
	inflightAdd(chatID, msgID)
}

// dropStartedNotice removes the "Download started!" notices of a finished task
// from every chat that received them.
func dropStartedNotice(gid string) {
	if gid == "" {
		return
	}
	startedNoticesMu.Lock()
	msgs, ok := startedNotices[gid]
	delete(startedNotices, gid)
	startedNoticesMu.Unlock()
	if ok {
		deleteChatMsgs(activeBot, msgs)
	}
}

// OnDownloadStart will be sent when a download is started. The event is of type struct, and it contains following keys. The value type is string.
func (Notifier) OnDownloadStart(events []rpc.Event) {
	logger.Info(i18nLoc.LocText("onDownloadStartDes"), events)

	if len(events) > 0 {
		gid := events[0].Gid
		SuddenMessageChan <- suddenMsg{GID: gid, Text: fmt.Sprintf(i18nLoc.LocText("onDownloadStartDes"), events)}
		aria2.TMMessageChan <- gid
		// show the live progress view automatically, no button press needed
		go autoShowProgress(gid)
	}
}

// autoShowProgress starts the live download progress message right after a
// download begins in every relevant chat (owner + admins, filtered views).
func autoShowProgress(gid string) {
	if activeBot == nil {
		return
	}
	// brief delay so the new task is registered as active in aria2 and the
	// taskStore ownership entry (written right after Download()) is visible
	time.Sleep(400 * time.Millisecond)
	chats := chatsForGid(gid)
	for _, chat := range dedupChats(chats) {
		var res string
		if isAdminID(chat) {
			res = input.ToolApp.Aria2.FormatTellActive()
		} else {
			res = input.ToolApp.Aria2.FormatTellActiveFiltered(allowGidsFor(chat))
		}
		if res == "" {
			continue
		}
		startActiveRefresh(chat, 0)
		// stagger so concurrent sends do not race the same aria2 RPC
		time.Sleep(100 * time.Millisecond)
	}
}

// OnDownloadPause will be sent when a download is paused. The event is the same struct as the event argument of onDownloadStart() method.
func (Notifier) OnDownloadPause(events []rpc.Event) {
	logger.Info(i18nLoc.LocText("onDownloadPauseDes"), events)
	if len(events) > 0 && aria2.TakeAutoPaused(events[0].Gid) {
		// picker auto-pause for file selection, not a user action - no noise
		return
	}
	if len(events) > 0 {
		SuddenMessageChan <- suddenMsg{GID: events[0].Gid, Text: fmt.Sprintf(i18nLoc.LocText("onDownloadPauseDes"), events)}
	} else {
		SuddenMessageChan <- suddenMsg{Text: fmt.Sprintf(i18nLoc.LocText("onDownloadPauseDes"), events)}
	}
}

// OnDownloadStop will be sent when a download is stopped by the user. The event is the same struct as the event argument of onDownloadStart() method.
func (Notifier) OnDownloadStop(events []rpc.Event) {
	logger.Info(i18nLoc.LocText("onDownloadStopDes"), events)
	if len(events) > 0 {
		SuddenMessageChan <- suddenMsg{GID: events[0].Gid, Text: fmt.Sprintf(i18nLoc.LocText("onDownloadStopDes"), events)}
	} else {
		SuddenMessageChan <- suddenMsg{Text: fmt.Sprintf(i18nLoc.LocText("onDownloadStopDes"), events)}
	}
	if len(events) > 0 {
		dropStartedNotice(events[0].Gid)
	}
}

// OnDownloadComplete will be sent when a download is complete. For BitTorrent downloads, this notification is sent when the download is complete and seeding is over. The event is the same struct of the event argument of onDownloadStart() method.
func (Notifier) OnDownloadComplete(events []rpc.Event) {
	logger.Info(i18nLoc.LocText("onDownloadCompleteDes"), events)
	if len(events) > 0 {
		aria2.MarkFinished(events[0].Gid)
		dropStartedNotice(events[0].Gid)
	}
	handleDownloadComplete(events)
}

// OnDownloadError will be sent when a download is stopped due to an error. The event is the same struct as the event argument of onDownloadStart() method.
func (Notifier) OnDownloadError(events []rpc.Event) {
	logger.Info(i18nLoc.LocText("onDownloadErrorDes"), events)
	if len(events) > 0 {
		SuddenMessageChan <- suddenMsg{GID: events[0].Gid, Text: fmt.Sprintf(i18nLoc.LocText("onDownloadErrorDes"), events)}
	} else {
		SuddenMessageChan <- suddenMsg{Text: fmt.Sprintf(i18nLoc.LocText("onDownloadErrorDes"), events)}
	}
	if len(events) > 0 {
		dropStartedNotice(events[0].Gid)
	}
}

// OnBtDownloadComplete will be sent when a torrent download is complete but seeding is still going on. The event is the same struct as the event argument of onDownloadStart() method.
func (Notifier) OnBtDownloadComplete(events []rpc.Event) {
	logger.Info(i18nLoc.LocText("onBtDownloadCompleteDes"), events)
	if len(events) > 0 {
		aria2.MarkFinished(events[0].Gid)
		dropStartedNotice(events[0].Gid)
	}
}
