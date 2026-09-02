// Crash-safe tracking of in-flight bot messages. If the process dies while
// an organize pipeline is running, its live progress message would linger in
// the chat forever. The IDs are persisted to a small state file so a restart
// can clean them up.
//
// Copyright 2026 spaulll - prox-download-bot (Apache-2.0)
package telegram

import (
	"encoding/json"
	"os"
	"sync"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	logger "DownloadBot/tool/zap"
)

const inflightFile = "inflight_msgs.json"

var inflightMu sync.Mutex

type inflightState struct {
	ChatID     int64   `json:"chat_id"`
	MessageIDs []int64 `json:"message_ids"`
}

func loadInflight() inflightState {
	var st inflightState
	b, err := os.ReadFile(inflightFile)
	if err != nil {
		return st
	}
	if json.Unmarshal(b, &st) != nil {
		// corrupted state: drop it
		_ = os.Remove(inflightFile)
	}
	return st
}

func saveInflight(st inflightState) {
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.WriteFile(inflightFile, b, 0o600)
}

// inflightAdd registers a message that must be cleaned up after a crash.
func inflightAdd(chatID int64, messageID int) {
	if messageID <= 0 {
		return
	}
	inflightMu.Lock()
	defer inflightMu.Unlock()
	st := loadInflight()
	st.ChatID = chatID
	for _, id := range st.MessageIDs {
		if id == int64(messageID) {
			return
		}
	}
	st.MessageIDs = append(st.MessageIDs, int64(messageID))
	saveInflight(st)
}

// inflightRemove unregisters a message that was handled (deleted or kept
// intentionally as a final summary).
func inflightRemove(messageID int) {
	if messageID <= 0 {
		return
	}
	inflightMu.Lock()
	defer inflightMu.Unlock()
	st := loadInflight()
	kept := st.MessageIDs[:0]
	for _, id := range st.MessageIDs {
		if id != int64(messageID) {
			kept = append(kept, id)
		}
	}
	st.MessageIDs = kept
	if len(st.MessageIDs) == 0 {
		_ = os.Remove(inflightFile)
		return
	}
	saveInflight(st)
}

// inflightCleanup deletes all messages left over from crashed runs.
// Runs once at startup, before any new pipeline can start.
func inflightCleanup(bot *tgBotApi.BotAPI) {
	inflightMu.Lock()
	defer inflightMu.Unlock()
	st := loadInflight()
	if len(st.MessageIDs) == 0 {
		return
	}
	logger.Info("recovery: cleaning %d in-flight message(s) from previous run", len(st.MessageIDs))
	for _, id := range st.MessageIDs {
		if id <= 0 {
			continue
		}
		if _, err := bot.Request(tgBotApi.NewDeleteMessage(st.ChatID, int(id))); err != nil {
			logger.Debug("inflight cleanup delete failed: %v", err)
		}
	}
	_ = os.Remove(inflightFile)
}
