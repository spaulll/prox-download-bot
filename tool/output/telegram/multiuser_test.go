package telegram

import (
	"os"
	"strings"
	"testing"

	i18nLoc "DownloadBot/i18n"
	"DownloadBot/internal/config"
	"DownloadBot/internal/users"
	logger "DownloadBot/tool/zap"
)

func TestMultiUserRouting(t *testing.T) {
	// setup globals
	adminIDs = []int64{1}
	taskStore = users.NewTaskStore(t.TempDir() + "/tasks.json")
	taskStore.Add(users.Task{GID: "g1", UserID: 100, ChatID: 100, Link: "http://x", Engine: "aria2"})
	taskStore.Add(users.Task{GID: "g2", UserID: 100, ChatID: 100, Link: "http://y", Engine: "aria2"})
	taskStore.Add(users.Task{GID: "g3", UserID: 200, ChatID: 200, Link: "http://z", Engine: "aria2"})

	// chatsForGidFast: owner + chat + admins
	chats := chatsForGidFast("g1")
	if len(chats) != 2 {
		t.Fatalf("chatsForGidFast(g1) = %v, want 2 entries (owner+admin)", chats)
	}
	has := map[int64]bool{}
	for _, c := range chats {
		has[c] = true
	}
	if !has[100] || !has[1] {
		t.Fatalf("chatsForGidFast(g1) = %v, want [100 1]", chats)
	}

	// unknown gid -> admins only
	chats = chatsForGidFast("unknown")
	if len(chats) != 1 || chats[0] != 1 {
		t.Fatalf("unknown gid chats = %v, want [1]", chats)
	}

	// allowGidsFor: admin nil (all), user filtered
	if allowGidsFor(1) != nil {
		t.Fatalf("admin allow should be nil (all)")
	}
	allow := allowGidsFor(100)
	if len(allow) != 2 || !allow["g1"] || !allow["g2"] {
		t.Fatalf("user 100 allow = %v, want g1,g2", allow)
	}
	allow = allowGidsFor(200)
	if len(allow) != 1 || !allow["g3"] {
		t.Fatalf("user 200 allow = %v, want g3", allow)
	}

	// canControlGid
	if !canControlGid(1, "g1") {
		t.Fatalf("admin should control g1")
	}
	if !canControlGid(100, "g1") {
		t.Fatalf("owner should control own gid")
	}
	if canControlGid(200, "g1") {
		t.Fatalf("other user must NOT control g1")
	}
	if canControlGid(100, "unknown") {
		t.Fatalf("non-admin must NOT control unknown gid")
	}

	// group chat: UserID != ChatID, both should receive
	taskStore.Add(users.Task{GID: "g4", UserID: 100, ChatID: 999, Link: "http://w", Engine: "aria2"})
	chats = chatsForGidFast("g4")
	has = map[int64]bool{}
	for _, c := range chats {
		has[c] = true
	}
	if !has[100] || !has[999] || !has[1] {
		t.Fatalf("group chats = %v, want [100 999 1]", chats)
	}

	// dedup: admin owner collapses
	adminIDs = []int64{1}
	taskStore.Add(users.Task{GID: "g5", UserID: 1, ChatID: 1, Link: "http://a", Engine: "aria2"})
	chats = chatsForGidFast("g5")
	if len(chats) != 1 || chats[0] != 1 {
		t.Fatalf("admin own chats = %v, want [1]", chats)
	}
}

func TestApprovedUsersList(t *testing.T) {
	i18nLoc.LocLan("en")
	userStore = users.Open(t.TempDir() + "/users.json")
	userStore.SetRole(1, users.RoleAdmin)
	// give users usernames via start records, then approve them
	userStore.UpsertStarted(100, "alice", "Alice A")
	userStore.SetRole(100, users.RoleApproved)
	userStore.UpsertStarted(101, "bob", "Bob B")
	userStore.SetRole(101, users.RoleApproved)
	userStore.SetRole(102, users.RoleDenied)

	if got := len(approvedUsers()); got != 2 {
		t.Fatalf("approvedUsers() = %d, want 2", got)
	}
	text, markup := buildApprovedUsersList()
	if markup == nil {
		t.Fatal("expected non-nil markup with approved users")
	}
	if !strings.Contains(text, "@alice") || !strings.Contains(text, "@bob") {
		t.Fatalf("list text missing users:\n%s", text)
	}
	if len(markup.InlineKeyboard) != 2 {
		t.Fatalf("markup rows = %d, want 2", len(markup.InlineKeyboard))
	}

	// simulate the remove callback: revoke + list refreshes
	userStore.SetRole(100, users.RoleDenied)
	if got := len(approvedUsers()); got != 1 {
		t.Fatalf("approvedUsers() after remove = %d, want 1", got)
	}
	text, _ = buildApprovedUsersList()
	if strings.Contains(text, "@alice") {
		t.Fatalf("removed user still listed:\n%s", text)
	}

	// underscore usernames must be markdown-escaped (unescaped '_' makes
	// Telegram reject the message, which panics the send path)
	userStore.UpsertStarted(103, "aniket_050", "A N")
	userStore.SetRole(103, users.RoleApproved)
	text, _ = buildApprovedUsersList()
	if !strings.Contains(text, `@aniket\_050`) {
		t.Fatalf("username not markdown-escaped:\n%s", text)
	}

	// empty list: no markup, friendly message
	userStore.SetRole(101, users.RoleDenied)
	userStore.SetRole(103, users.RoleDenied)
	text, markup = buildApprovedUsersList()
	if markup != nil {
		t.Fatal("expected nil markup with no approved users")
	}
	if !strings.Contains(text, i18nLoc.LocText("noApprovedUsers")) {
		t.Fatalf("empty list text wrong:\n%s", text)
	}
}

func TestRepairStaleAdmins(t *testing.T) {
	i18nLoc.LocLan("en")
	logger.InitLog("", "", "info")
	adminIDs = []int64{1}
	userStore = users.Open(t.TempDir() + "/users.json")
	// legacy stale record: regular user stored as RoleAdmin (0)
	userStore.SetRole(1, users.RoleAdmin)
	userStore.SetRole(200, users.RoleAdmin)
	if got := len(approvedUsers()); got != 0 {
		t.Fatalf("approvedUsers() before repair = %d, want 0 (stale admin invisible)", got)
	}
	repairStaleAdmins()
	if u, _ := userStore.Get(200); u.Role != users.RoleApproved {
		t.Fatalf("stale admin role = %v, want approved", users.RoleName(u.Role))
	}
	if u, _ := userStore.Get(1); u.Role != users.RoleAdmin {
		t.Fatalf("real admin role = %v, want admin", users.RoleName(u.Role))
	}
	if got := len(approvedUsers()); got != 1 {
		t.Fatalf("approvedUsers() after repair = %d, want 1", got)
	}
	text, markup := buildApprovedUsersList()
	if markup == nil || !strings.Contains(text, "200") {
		t.Fatalf("repaired user missing from list:\n%s", text)
	}
}

func TestTelegramFileHelpers(t *testing.T) {
	if !isTorrentUpload("movie.torrent", "") {
		t.Error("movie.torrent should be a torrent upload")
	}
	if !isTorrentUpload("MOVIE.TORRENT", "") {
		t.Error("upper-case .torrent should be a torrent upload")
	}
	if !isTorrentUpload("x.bin", "application/x-bittorrent") {
		t.Error("bittorrent mime should be a torrent upload")
	}
	if isTorrentUpload("movie.mkv", "video/x-matroska") {
		t.Error("movie.mkv must not be a torrent upload")
	}
	for in, want := range map[string]string{
		"Some Show S01E01.mkv": "Some Show S01E01.mkv",
		"/etc/passwd":          "passwd",
		`..\windows\evil.exe`:  "evil.exe",
		"  spaced.mp4  ":        "spaced.mp4",
		"":                     "",
		".":                    "",
	} {
		if got := safeOutName(in); got != want {
			t.Errorf("safeOutName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalBotAPIConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"input":{"aria2":{"aria2-server":"ws://127.0.0.1:6800/jsonrpc","aria2-key":"x"}},` +
		`"output":{"telegram":{"bot-key":"k","user-id":"1","api-id":123,"api-hash":"h","api-base":"http://127.0.0.1:8081/"}},` +
		`"max-index":10,"language":"en","downloadFolder":"/tmp","organize":{"enabled":false},"log":{"level":"info"}}`
	if err := os.WriteFile(dir+"/c.json", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	config.InitConfig(dir + "/c.json")
	if got := config.GetTelegramApiBase(); got != "http://127.0.0.1:8081" {
		t.Fatalf("api-base = %q, want trimmed host without trailing slash", got)
	}
	if config.GetTelegramApiID() != 123 || config.GetTelegramApiHash() != "h" {
		t.Fatal("api-id/api-hash not loaded")
	}
	if got := telegramDownloadCap(); got != 2000*1024*1024 {
		t.Fatalf("local cap = %d, want 2GB", got)
	}
}

func TestTelegramFileURL(t *testing.T) {
	botToken = "TESTTOKEN"
	t.Cleanup(func() { botToken = "" })
	// cloud mode (config from TestLocalBotAPIConfig may persist; force cloud
	// by testing the format branches directly)
	if got := telegramFileURL("docs/f.bin"); !strings.Contains(got, "TESTTOKEN") || !strings.Contains(got, "docs/f.bin") {
		t.Fatalf("file url = %q, want token + path", got)
	}
}
