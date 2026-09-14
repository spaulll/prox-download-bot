// Multi-user delivery helpers: every download/organize progress message must
// reach BOTH the task owner and all admins (deduplicated). Regular users only
// ever see their own tasks; admins see everything.
//
// Copyright 2026 spaulll - prox-download-bot (Apache-2.0)
package telegram

import (
	"time"

	tgBotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"DownloadBot/tool/input"
	logger "DownloadBot/tool/zap"
)

// chatMsgID pairs a Telegram chat with one message ID in that chat.
type chatMsgID struct {
	ChatID int64
	MsgID  int
}

// adminChatIDs returns all configured admin chat IDs.
func adminChatIDs() []int64 {
	return append([]int64(nil), adminIDs...)
}

// primaryAdminID returns the first admin (legacy single-admin paths).
func primaryAdminID() int64 {
	if len(adminIDs) > 0 {
		return adminIDs[0]
	}
	return 0
}

// dedupChats removes duplicates and zeros, preserving order.
func dedupChats(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// ownerOfGid returns the user that added gid.
func ownerOfGid(gid string) (int64, bool) {
	if taskStore == nil || gid == "" {
		return 0, false
	}
	t, ok := taskStore.Get(gid)
	if !ok {
		return 0, false
	}
	return t.UserID, true
}

// taskChats returns owner + request chat + admins for a task.
func taskChats(userID, chatID int64) []int64 {
	ids := []int64{}
	if userID != 0 {
		ids = append(ids, userID)
	}
	if chatID != 0 && chatID != userID {
		ids = append(ids, chatID)
	}
	for _, a := range adminIDs {
		skip := false
		for _, have := range ids {
			if have == a {
				skip = true
				break
			}
		}
		if !skip {
			ids = append(ids, a)
		}
	}
	return dedupChats(ids)
}

// waitOwnerOfGid polls briefly for the task entry. Download() returns the gid
// and only then records ownership, while aria2's onDownloadStart event fires
// immediately - without this the first "Download started!" notice would miss
// the owner and go to the admin only.
func waitOwnerOfGid(gid string) (int64, bool) {
	if id, ok := ownerOfGid(gid); ok {
		return id, true
	}
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		if id, ok := ownerOfGid(gid); ok {
			return id, true
		}
	}
	return 0, false
}

// chatsForOwner returns owner + all admins deduplicated. When the owner is an
// admin the result collapses to the admin set (no duplicate messages).
func chatsForOwner(owner int64) []int64 {
	if owner == 0 {
		return adminChatIDs()
	}
	ids := []int64{owner}
	for _, a := range adminIDs {
		if a != owner {
			ids = append(ids, a)
		}
	}
	return dedupChats(ids)
}

// chatsForGid returns the delivery targets for a task gid: owner + request
// chat + admins. Unknown gids fall back to admins only (previous behavior).
func chatsForGid(gid string) []int64 {
	if taskStore != nil && gid != "" {
		for i := 0; i < 10; i++ {
			if t, ok := taskStore.Get(gid); ok {
				return taskChats(t.UserID, t.ChatID)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	return adminChatIDs()
}

// chatsForGidFast is the non-blocking variant for hot paths (progress ticks)
// where the entry is expected to already exist.
func chatsForGidFast(gid string) []int64 {
	if taskStore != nil && gid != "" {
		if t, ok := taskStore.Get(gid); ok {
			return taskChats(t.UserID, t.ChatID)
		}
	}
	return adminChatIDs()
}

// allowGidsFor returns the aria2 gid filter for a user: nil for admins (show
// everything), otherwise the set of gids owned by that user.
func allowGidsFor(userID int64) map[string]bool {
	if isAdminID(userID) {
		return nil
	}
	if taskStore == nil {
		return map[string]bool{}
	}
	allow := map[string]bool{}
	for _, t := range taskStore.ByUser(userID) {
		allow[t.GID] = true
	}
	return allow
}

// canControlGid reports whether userID may act on gid (pause/resume/remove,
// torrent file selection). Admins may control everything; regular users only
// their own tasks. Unknown gids are admin-only so users cannot probe others.
func canControlGid(userID int64, gid string) bool {
	if isAdminID(userID) {
		return true
	}
	if taskStore == nil || gid == "" {
		return false
	}
	t, ok := taskStore.Get(gid)
	if !ok {
		return false
	}
	return t.UserID == userID
}

// markTaskCompleted records completion without sending a duplicate notice.
// Callers already mirror the "Download completed" message to all chats.
func markTaskCompleted(gid string) {
	if taskStore == nil || gid == "" {
		return
	}
	taskStore.SetStatus(gid, "completed")
}

// pauseOwnTasks pauses only the tasks owned by userID (non-admin bulk action).
func pauseOwnTasks(userID int64) {
	if taskStore == nil {
		return
	}
	for _, t := range taskStore.ByUser(userID) {
		if t.Engine == "ytdlp" {
			continue
		}
		input.ToolApp.Aria2.Pause(t.GID)
	}
}

// resumeOwnTasks resumes only the tasks owned by userID (non-admin bulk action).
func resumeOwnTasks(userID int64) {
	if taskStore == nil {
		return
	}
	for _, t := range taskStore.ByUser(userID) {
		if t.Engine == "ytdlp" {
			continue
		}
		input.ToolApp.Aria2.Unpause(t.GID)
	}
}

// sendToChats sends text to every chat, returning per-chat message IDs.
// Failures are logged and skipped (one blocked user must not break the other).
func sendToChats(bot *tgBotApi.BotAPI, chats []int64, text string) []chatMsgID {
	out := make([]chatMsgID, 0, len(chats))
	for _, chat := range chats {
		id := sendPlain(bot, chat, text)
		if id != 0 {
			out = append(out, chatMsgID{ChatID: chat, MsgID: id})
		}
	}
	return out
}

// trackChatMsgs registers sent messages for crash cleanup.
func trackChatMsgs(msgs []chatMsgID) {
	for _, m := range msgs {
		inflightAdd(m.ChatID, m.MsgID)
	}
}

// deleteChatMsgs deletes messages per chat and untracks them.
func deleteChatMsgs(bot *tgBotApi.BotAPI, msgs []chatMsgID) {
	if bot == nil {
		return
	}
	for _, m := range msgs {
		if m.MsgID == 0 {
			continue
		}
		if _, err := bot.Request(tgBotApi.NewDeleteMessage(m.ChatID, m.MsgID)); err != nil {
			logger.Debug("delete message failed: %v", err)
		}
		inflightRemoveChat(m.ChatID, m.MsgID)
	}
}

// DualProgressMsg mirrors a live-updating message into several chats (owner +
// admins). It wraps one OrganizeProgressMsg per chat so edits/deletes stay in
// sync without changing the single-chat type.
type DualProgressMsg struct {
	msgs []*OrganizeProgressMsg
}

// NewDualProgressMsg sends the initial text to every chat.
func NewDualProgressMsg(bot *tgBotApi.BotAPI, chats []int64, text string) *DualProgressMsg {
	d := &DualProgressMsg{}
	for _, chat := range dedupChats(chats) {
		if m := NewOrganizeProgressMsg(bot, chat, text); m != nil {
			d.msgs = append(d.msgs, m)
		}
	}
	if len(d.msgs) == 0 {
		return nil
	}
	return d
}

// Update edits every mirrored message.
func (d *DualProgressMsg) Update(text string) {
	if d == nil {
		return
	}
	for _, m := range d.msgs {
		m.Update(text)
	}
}

// Delete removes every mirrored message.
func (d *DualProgressMsg) Delete() {
	if d == nil {
		return
	}
	for _, m := range d.msgs {
		m.Delete()
	}
}

// Untrack keeps the messages but drops crash-cleanup tracking (used when the
// live message becomes the final summary).
func (d *DualProgressMsg) Untrack() {
	if d == nil {
		return
	}
	for _, m := range d.msgs {
		inflightRemoveChat(m.chatID, m.messageID)
	}
}

// Chats returns the chat IDs backing this dual message.
func (d *DualProgressMsg) Chats() []int64 {
	if d == nil {
		return nil
	}
	out := make([]int64, 0, len(d.msgs))
	for _, m := range d.msgs {
		out = append(out, m.chatID)
	}
	return out
}
