package controller

import (
	"DownloadBot/internal/config"
	logger "DownloadBot/tool/zap"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

func RemoveFiles(deleteFiles []string) {
	for _, removePath := range deleteFiles {
		if removePath != config.GetDownloadFolder() && removePath != config.GetDownloadFolder()+"/" {
			err := os.RemoveAll(removePath)
			logger.DropErr(err)
		}
	}
}

// CopyFiles copies files from the download folder to the same relative path in the library
func CopyFiles(srcFiles []string, sendAutoUpdateMessage func(text string)) {
	destPath := config.GetDownloadFolder()
	downloadFolder := config.GetDownloadFolder()
	if !strings.HasSuffix(destPath, "/") {
		destPath += "/"
	}
	if !strings.HasSuffix(downloadFolder, "/") {
		downloadFolder += "/"
	}
	newMsg := sendAutoUpdateMessage
	for _, srcPath := range srcFiles {
		if srcPath != config.GetDownloadFolder() && srcPath != config.GetDownloadFolder()+"/" {
			file1, err := os.Open(srcPath)
			logger.DropErr(err)
			s, err := os.Stat(srcPath)
			if err == nil {
				if s.IsDir() {
					_, err := os.Stat(strings.ReplaceAll(srcPath, downloadFolder, destPath))
					if err != nil {
						err = os.MkdirAll(strings.ReplaceAll(srcPath, downloadFolder, destPath), os.ModePerm)
						logger.DropErr(err)
					} else {
						continue
					}
				} else {
					paths, _ := filepath.Split(strings.ReplaceAll(srcPath, downloadFolder, destPath))
					_, err := os.Stat(paths)
					if err != nil {
						err = os.MkdirAll(paths, os.ModePerm)
						logger.DropErr(err)
					}
				}
			}

			file2, err := os.OpenFile(strings.ReplaceAll(srcPath, downloadFolder, destPath), os.O_WRONLY|os.O_CREATE, os.ModePerm)
			logger.DropErr(err)
			defer file1.Close()
			defer file2.Close()
			bs := make([]byte, 1024, 1024)
			n := -1 // bytes read
			for {
				n, err = file1.Read(bs)
				if err == io.EOF || n == 0 {
					break
				}
				logger.DropErr(err)
				_, err = file2.Write(bs[:n])
			}
		}
	}
	newMsg("close")
}

// RandStringRunes Generate random string
func RandStringRunes(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
