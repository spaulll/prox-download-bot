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

type inflightEntry struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int64 `json:"message_id"`
}

type inflightState struct {
	// new multi-chat format
	Entries []inflightEntry `json:"entries,omitempty"`
	// legacy single-chat format (chat_id + message_ids)
	ChatID     int64   `json:"chat_id,omitempty"`
	MessageIDs []int64 `json:"message_ids,omitempty"`
}

func loadInflightEntries() []inflightEntry {
	var st inflightState
	b, err := os.ReadFile(inflightFile)
	if err != nil {
		return nil
	}
	if json.Unmarshal(b, &st) != nil {
		// corrupted state: drop it
		_ = os.Remove(inflightFile)
		return nil
	}
	if len(st.Entries) > 0 {
		return st.Entries
	}
	if len(st.MessageIDs) > 0 {
		out := make([]inflightEntry, 0, len(st.MessageIDs))
		for _, id := range st.MessageIDs {
			out = append(out, inflightEntry{ChatID: st.ChatID, MessageID: id})
		}
		return out
	}
	return nil
}

func saveInflightEntries(entries []inflightEntry) {
	if len(entries) == 0 {
		_ = os.Remove(inflightFile)
		return
	}
	b, err := json.Marshal(inflightState{Entries: entries})
	if err != nil {
		return
	}
	_ = os.WriteFile(inflightFile, b, 0o600)
}

// inflightAdd registers a message that must be cleaned up after a crash.
func inflightAdd(chatID int64, messageID int) {
	if messageID <= 0 || chatID == 0 {
		return
	}
	inflightMu.Lock()
	defer inflightMu.Unlock()
	entries := loadInflightEntries()
	for _, e := range entries {
		if e.ChatID == chatID && e.MessageID == int64(messageID) {
			return
		}
	}
	entries = append(entries, inflightEntry{ChatID: chatID, MessageID: int64(messageID)})
	saveInflightEntries(entries)
}

// inflightRemoveChat unregisters one chat message that was handled (deleted
// or kept intentionally as a final summary).
func inflightRemoveChat(chatID int64, messageID int) {
	if messageID <= 0 {
		return
	}
	inflightMu.Lock()
	defer inflightMu.Unlock()
	entries := loadInflightEntries()
	kept := entries[:0]
	for _, e := range entries {
		if e.MessageID == int64(messageID) && (chatID == 0 || e.ChatID == chatID) {
			continue
		}
		kept = append(kept, e)
	}
	saveInflightEntries(kept)
}

// inflightRemove unregisters a message ID in every chat (legacy helper kept
// for call sites that only know the message ID).
func inflightRemove(messageID int) {
	inflightRemoveChat(0, messageID)
}

// inflightCleanup deletes all messages left over from crashed runs.
// Runs once at startup, before any new pipeline can start.
func inflightCleanup(bot *tgBotApi.BotAPI) {
	inflightMu.Lock()
	defer inflightMu.Unlock()
	entries := loadInflightEntries()
	if len(entries) == 0 {
		return
	}
	logger.Info("recovery: cleaning %d in-flight message(s) from previous run", len(entries))
	for _, e := range entries {
		if e.MessageID <= 0 || e.ChatID == 0 {
			continue
		}
		if _, err := bot.Request(tgBotApi.NewDeleteMessage(e.ChatID, int(e.MessageID))); err != nil {
			logger.Debug("inflight cleanup delete failed: %v", err)
		}
	}
	_ = os.Remove(inflightFile)
}
