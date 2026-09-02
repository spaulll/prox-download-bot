package telegram

import (
	i18nLoc "DownloadBot/i18n"
	"DownloadBot/internal/config"
	"DownloadBot/tool/controller"
	"DownloadBot/tool/displayUtil/gotree"
	"DownloadBot/tool/typeTrans"
	"fmt"
	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"io/ioutil"
	"path"
	"strings"
)

// FileControlChan is a channel for controlling files,including deleting files, copying files, uploading files
var FileControlChan = make(chan string, 3)

// removeFilesPrint is a function that deletes files
func removeFilesPrint(chatMsgID int, bot *tgBotApi.BotAPI) {
	s := <-FileControlChan
	if s == "file" {
		FileControlChan <- "file"
	}
	var MessageID = 0
	var filesSelect = make(map[int]bool)
	fileList, _ := getAllFile(config.GetDownloadFolder())
	myID := typeTrans.Str2Int64(config.GetTelegramUserID())

	msgToDelete := tgBotApi.DeleteMessageConfig{
		ChatID:    myID,
		MessageID: chatMsgID,
	}
	_, _ = bot.Request(msgToDelete)

	if len(fileList) == 1 {
		bot.Send(tgBotApi.NewMessage(myID, i18nLoc.LocText("noFilesFound")))
		return
	}
	deleteFiles := make([]string, 0)
	for {
		a := <-FileControlChan
		if a == "close" {
			//tgBotApi.NewDeleteMessage(myID, MessageID)
			bot.Send(tgBotApi.NewDeleteMessage(myID, MessageID))
			return
		}
		b := strings.Split(a, "~")
		fileTree := ""
		if len(b) == 1 {
			filesSelect = make(map[int]bool)
			for i := 1; i <= len(fileList); i++ {
				filesSelect[i] = true
			}
			fileTree, filesSelect, deleteFiles = printFolderTree(config.GetDownloadFolder(), filesSelect, "0")
		} else {
			if b[1] == "cancel" {
				tgBotApi.NewDeleteMessage(myID, MessageID)
				bot.Send(tgBotApi.NewDeleteMessage(myID, MessageID))
				return
			} else if b[1] == "Delete" {
				controller.RemoveFiles(deleteFiles)
				bot.Send(tgBotApi.NewDeleteMessage(myID, MessageID))
				bot.Send(tgBotApi.NewMessage(myID, i18nLoc.LocText("filesDeletedSuccessfully")))
				return
			}
			fileTree, filesSelect, deleteFiles = printFolderTree(config.GetDownloadFolder(), filesSelect, b[1])
		}

		text := fmt.Sprintf("%s %s\n", config.GetDownloadFolder(), i18nLoc.LocText("fileDirectoryIsAsFollows")) + fileTree
		Keyboards := make([][]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow := make([]tgBotApi.InlineKeyboardButton, 0)
		index := 1
		for _, _ = range fileList {
			inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(fmt.Sprint(index), "file~"+fmt.Sprint(index)+":8"))
			if index%7 == 0 {
				Keyboards = append(Keyboards, inlineKeyBoardRow)
				inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
			}
			index++
		}
		text += i18nLoc.LocText("pleaseSelectTheFileYouWantToDelete")
		if len(inlineKeyBoardRow) != 0 {
			Keyboards = append(Keyboards, inlineKeyBoardRow)
		}
		inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("selectAll"), "file~selectAll"+":9"))
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("invert"), "file~invert"+":9"))
		Keyboards = append(Keyboards, inlineKeyBoardRow)
		inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("confirmDelete"), "file~Delete"+":9"))
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("cancel"), "file~cancel"+":9"))
		Keyboards = append(Keyboards, inlineKeyBoardRow)

		msg := tgBotApi.NewMessage(myID, text)
		if MessageID == 0 {
			msg.ReplyMarkup = tgBotApi.NewInlineKeyboardMarkup(Keyboards...)
			res, err := bot.Send(msg)
			dropErr(err)
			MessageID = res.MessageID

		} else {
			newMsg := tgBotApi.NewEditMessageTextAndMarkup(myID, MessageID, text, tgBotApi.NewInlineKeyboardMarkup(Keyboards...))
			bot.Send(newMsg)
		}
	}
}

func copyFilesPrint(chatMsgID int, bot *tgBotApi.BotAPI) {
	s := <-FileControlChan
	if s == "file" {
		FileControlChan <- "file"
	}
	var MessageID = 0
	var filesSelect = make(map[int]bool)
	fileList, _ := getAllFile(config.GetDownloadFolder())
	myID := typeTrans.Str2Int64(config.GetTelegramUserID())

	msgToDelete := tgBotApi.DeleteMessageConfig{
		ChatID:    myID,
		MessageID: chatMsgID,
	}
	_, _ = bot.Request(msgToDelete)

	if len(fileList) == 1 {
		bot.Send(tgBotApi.NewMessage(myID, i18nLoc.LocText("noFilesFound")))
		return
	}
	copyFiles := make([]string, 0)
	for {
		a := <-FileControlChan
		if a == "close" {
			//tgBotApi.NewDeleteMessage(myID, MessageID)
			bot.Send(tgBotApi.NewDeleteMessage(myID, MessageID))
			return
		}
		b := strings.Split(a, "~")
		fileTree := ""
		if len(b) == 1 {
			filesSelect = make(map[int]bool)
			for i := 1; i <= len(fileList); i++ {
				filesSelect[i] = true
			}
			fileTree, filesSelect, copyFiles = printFolderTree(config.GetDownloadFolder(), filesSelect, "0")
		} else {
			if b[1] == "cancel" {
				//tgBotApi.NewDeleteMessage(myID, MessageID)
				bot.Send(tgBotApi.NewDeleteMessage(myID, MessageID))
				return
			} else if b[1] == "Copy" {
				controller.CopyFiles(copyFiles, SendTelegramAutoUpdateMessage())
				bot.Send(tgBotApi.NewDeleteMessage(myID, MessageID))
				bot.Send(tgBotApi.NewMessage(myID, i18nLoc.LocText("filesCopySuccessfully")))
				return
			}
			fileTree, filesSelect, copyFiles = printFolderTree(config.GetDownloadFolder(), filesSelect, b[1])
		}

		text := fmt.Sprintf("%s %s\n", config.GetDownloadFolder(), i18nLoc.LocText("fileDirectoryIsAsFollows")) + fileTree
		Keyboards := make([][]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow := make([]tgBotApi.InlineKeyboardButton, 0)
		index := 1
		for _, _ = range fileList {
			inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(fmt.Sprint(index), "file~"+fmt.Sprint(index)+":8"))
			if index%7 == 0 {
				Keyboards = append(Keyboards, inlineKeyBoardRow)
				inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
			}
			index++
		}
		text += i18nLoc.LocText("pleaseSelectTheFileYouWantToCopy")
		if len(inlineKeyBoardRow) != 0 {
			Keyboards = append(Keyboards, inlineKeyBoardRow)
		}
		inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("selectAll"), "file~selectAll"+":9"))
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("invert"), "file~invert"+":9"))
		Keyboards = append(Keyboards, inlineKeyBoardRow)
		inlineKeyBoardRow = make([]tgBotApi.InlineKeyboardButton, 0)
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("confirmCopy"), "file~Copy"+":9"))
		inlineKeyBoardRow = append(inlineKeyBoardRow, tgBotApi.NewInlineKeyboardButtonData(i18nLoc.LocText("cancel"), "file~cancel"+":9"))
		Keyboards = append(Keyboards, inlineKeyBoardRow)

		msg := tgBotApi.NewMessage(myID, text)
		if MessageID == 0 {
			msg.ReplyMarkup = tgBotApi.NewInlineKeyboardMarkup(Keyboards...)
			res, err := bot.Send(msg)
			dropErr(err)
			MessageID = res.MessageID

		} else {
			newMsg := tgBotApi.NewEditMessageTextAndMarkup(myID, MessageID, text, tgBotApi.NewInlineKeyboardMarkup(Keyboards...))
			bot.Send(newMsg)
		}
	}
}

func getAllFile(pathname string) ([][]string, int64) {
	if pathname[:len(pathname)-1] != "/" {
		pathname += "/"
	}
	rd, err := ioutil.ReadDir(pathname)
	dropErr(err)
	res := make([][]string, 0)
	//
	totalSize := int64(0)
	for _, fi := range rd {
		if fi.IsDir() {
			ret, subSize := getAllFile(pathname + fi.Name())
			totalSize += subSize
			res = append(res, ret...)
		} else {
			var fileNameAndSize = []string{pathname + fmt.Sprintln(fi.Name()), fmt.Sprint(fi.Size())}
			res = append(res, fileNameAndSize)
			totalSize += fi.Size()
		}
	}
	var fileNameAndSize = []string{pathname + fmt.Sprintln(pathname), fmt.Sprint(totalSize)}
	res = append(res, fileNameAndSize)
	return res, totalSize
}

// generateFolderTree is a function that generate the directory tree and can give the file actually selected by the user according to the user's choice.
// @return a Tree, all file name, true or false selected according to all file of order num, list of paths for all files to be deleted
func generateFolderTree(pathname string, boot int, fileSelect map[int]bool, selectFileIndex string, parentSelected int8) (goTree.Tree, [][]string, map[int]bool, []string) {
	index := boot
	if pathname[:len(pathname)-1] != "/" {
		pathname += "/"
	}
	rd, err := ioutil.ReadDir(pathname)
	dropErr(err)
	res := make([][]string, 0)
	totalSize := 0
	treeFolder := make([]goTree.Tree, 0)
	treeFiles := make([]string, 0)
	trueFileSelect := make(map[int]bool, 0)
	subList := make(map[int]bool)
	subFilesPath := make([]string, 0)
	deleteFiles := make([]string, 0)
	var artist goTree.Tree
	bootSelect := int8(0)
	if selectFileIndex == fmt.Sprint(boot) || parentSelected != 0 {
		if fileSelect[boot] || parentSelected == -1 {
			bootSelect = -1 // nothing below selected
			trueFileSelect[boot] = false
			artist = goTree.New(fmt.Sprintf("⬜%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
		} else {
			bootSelect = 1 // everything below selected
			trueFileSelect[boot] = true
			artist = goTree.New(fmt.Sprintf("✅%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
		}
	}
	for _, fi := range rd {
		//log.Println(fi.Name())
		if fi.IsDir() {
			index++
			ret, filesInfo, subTrueFileSelect, subDeleteFiles := generateFolderTree(pathname+fi.Name(), index, fileSelect, selectFileIndex, bootSelect)
			subFolderSize := 0
			for _, iSize := range filesInfo {
				subFolderSize += typeTrans.Str2Int(iSize[1])
			}
			for _, iPath := range subDeleteFiles {
				deleteFiles = append(deleteFiles, iPath)
			}
			var folderNameAndSize = []string{pathname + fi.Name(), fmt.Sprint(subFolderSize)}
			res = append(res, folderNameAndSize)
			res = append(res, filesInfo...)
			treeFolder = append(treeFolder, ret)
			totalSize += subFolderSize
			for k, v := range subTrueFileSelect {

				trueFileSelect[k] = v
			}

			subList[index] = subTrueFileSelect[index]
			index += len(filesInfo)
		} else {
			treeFiles = append(treeFiles, fmt.Sprintf("%s * %s", fi.Name(), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fi.Size())))))
			subFilesPath = append(subFilesPath, pathname+fmt.Sprint(fi.Name()))
			var fileNameAndSize = []string{pathname + fmt.Sprint(fi.Name()), fmt.Sprint(fi.Size())}
			res = append(res, fileNameAndSize)
			totalSize += int(fi.Size())
		}
	}

	tempIndex := index
	for _, _ = range treeFiles {
		index++
		if selectFileIndex == "selectAll" {
			trueFileSelect[index] = true
		} else if selectFileIndex == "invert" || selectFileIndex == fmt.Sprint(index) {
			trueFileSelect[index] = !fileSelect[index]
		} else if bootSelect == 1 {
			trueFileSelect[index] = true
		} else if bootSelect == -1 {
			trueFileSelect[index] = false
		} else {
			trueFileSelect[index] = fileSelect[index]
		}
		subList[index] = trueFileSelect[index]
	}
	index = tempIndex

	if selectFileIndex == "selectAll" {
		artist = goTree.New(fmt.Sprintf("✅%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
		trueFileSelect[boot] = true
	} else if selectFileIndex == "invert" {
		artist = goTree.New(fmt.Sprintf("⬜%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
		trueFileSelect[boot] = false
	} else if _, al := trueFileSelect[typeTrans.Str2Int(selectFileIndex)]; al {
		artist = goTree.New(fmt.Sprintf("⬜%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
		if fileSelect[boot] {
			trueFileSelect[boot] = false
		} else {
			if !trueFileSelect[typeTrans.Str2Int(selectFileIndex)] {
				trueFileSelect[boot] = false
			} else {
				selectAllOthers := true
				for k, v := range subList {
					if k == typeTrans.Str2Int(selectFileIndex) {
						continue
					}
					if !v {
						selectAllOthers = false
						break
					}
				}
				if selectAllOthers {
					trueFileSelect[boot] = true
					artist = goTree.New(fmt.Sprintf("✅%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
				} else {
					trueFileSelect[boot] = false
				}
			}
		}
	} else if selectFileIndex == fmt.Sprint(boot) || parentSelected != 0 {
		if fileSelect[boot] || parentSelected == -1 {
			artist = goTree.New(fmt.Sprintf("⬜%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
		} else {
			artist = goTree.New(fmt.Sprintf("✅%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
		}
	} else {
		if fileSelect[boot] {
			artist = goTree.New(fmt.Sprintf("✅%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
			trueFileSelect[boot] = true
		} else {
			artist = goTree.New(fmt.Sprintf("⬜%d:%s * %s", boot, path.Base(pathname), typeTrans.Byte2Readable(typeTrans.Str2Float64(fmt.Sprint(fmt.Sprint(totalSize))))))
			trueFileSelect[boot] = false
		}
	}

	for _, val := range treeFolder {
		artist.AddTree(val)
	}
	for i, val := range treeFiles {
		index++
		if trueFileSelect[index] {
			artist.Add(fmt.Sprintf("✅%d:%s", index, val))
			deleteFiles = append(deleteFiles, subFilesPath[i])
		} else {
			artist.Add(fmt.Sprintf("⬜%d:%s", index, val))
		}
	}
	if trueFileSelect[boot] {
		deleteFiles = append(deleteFiles, pathname)
	}
	allFalse := true
	for _, v := range trueFileSelect {
		if v {
			allFalse = false
			break
		}
	}
	if boot == 1 && allFalse {
		//log.Println("ss")
		return generateFolderTree(pathname, 1, fileSelect, "0", int8(0))
	} else {
		return artist, res, trueFileSelect, deleteFiles
	}

}
func printFolderTree(pathName string, fileSelect map[int]bool, selectFileIndex string) (string, map[int]bool, []string) {
	tree, _, trueFileSelect, deleteFiles := generateFolderTree(pathName, 1, fileSelect, selectFileIndex, int8(0))
	return tree.Print(), trueFileSelect, deleteFiles
}
