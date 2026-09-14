package telegram

import (
	"os"
	"strings"
	"testing"
	"time"

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

	// underscore names must be markdown-escaped (unescaped '_' makes
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
		"  spaced.mp4  ":       "spaced.mp4",
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

func TestLocalFetchHelpers(t *testing.T) {
	if got := localFetchTimeout(0); got != 30*time.Minute {
		t.Fatalf("unknown-size timeout = %v, want 30m", got)
	}
	if got := localFetchTimeout(1024); got != 10*time.Minute {
		t.Fatalf("small-file timeout = %v, want 10m floor", got)
	}
	if got := localFetchTimeout(10 * 1024 * 1024 * 1024); got != 3*time.Hour {
		t.Fatalf("huge-file timeout = %v, want 3h cap", got)
	}
}

func TestLinkOrCopy(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/src.bin"
	if err := os.WriteFile(src, []byte("hello-telegram"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := dir + "/sub/dst.bin"
	if err := linkOrCopy(src, dst); err != nil {
		t.Fatalf("linkOrCopy: %v", err)
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != "hello-telegram" {
		t.Fatalf("dst content = %q, err = %v", b, err)
	}
	// server-copy cleanup must not destroy staged data
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(dst); err != nil || string(b) != "hello-telegram" {
		t.Fatalf("dst after src remove = %q, err = %v", b, err)
	}
}

func TestFormatLocalActive(t *testing.T) {
	i18nLoc.LocLan("en")
	logger.InitLog("", "", "info")
	adminIDs = []int64{1}
	registerLocalFetch("tg1", "a_show.mkv", 100, 100, 0, nil, nil)
	registerLocalFetch("tg2", "b_movie.mp4", 200, 200, 0, nil, nil)
	t.Cleanup(func() {
		unregisterLocalFetch("tg1")
		unregisterLocalFetch("tg2")
	})
	all := formatLocalActive(1)
	if !strings.Contains(all, "`a_show.mkv`") || !strings.Contains(all, "`b_movie.mp4`") {
		t.Fatalf("admin should see all fetches:\n%s", all)
	}
	own := formatLocalActive(100)
	if !strings.Contains(own, "`a_show.mkv`") || strings.Contains(own, "b_movie") {
		t.Fatalf("user 100 should see only own fetch:\n%s", own)
	}
	if got := formatLocalActive(999); got != "" {
		t.Fatalf("stranger should see nothing, got:\n%s", got)
	}
	// underscore names must be markdown-escaped (list sends with Markdown)
	registerLocalFetch("tg3", "my_file_name.mkv", 100, 100, 0, nil, nil)
	defer unregisterLocalFetch("tg3")
	if got := formatLocalActive(100); !strings.Contains(got, "`my_file_name.mkv`") {
		t.Fatalf("fetch name mangled, want raw name in backticks:\n%s", got)
	}
	// names sit inside backticks (literal span): escaping would show raw, so
	// only backticks themselves are stripped, nothing escaped
	registerLocalFetch("tg4", "evil`code.mkv", 100, 100, 0, nil, nil)
	defer unregisterLocalFetch("tg4")
	if got := formatLocalActive(100); strings.Contains(got, "`evil`") || !strings.Contains(got, "evilcode.mkv") {
		t.Fatalf("backtick in name not neutralized:\n%s", got)
	}
}

func TestTelegramHistoryAndCancel(t *testing.T) {
	i18nLoc.LocLan("en")
	logger.InitLog("", "", "info")
	adminIDs = []int64{1}
	taskStore = users.NewTaskStore(t.TempDir() + "/tasks.json")
	taskStore.Add(users.Task{GID: "tg9", UserID: 100, ChatID: 100, Link: "tg:file:show.mkv", Name: "show.mkv", Engine: "telegram", Status: "downloading"})
	taskStore.SetSize("tg9", 1024)
	taskStore.SetStatus("tg9", "completed")
	if task, _ := taskStore.Get("tg9"); task.FinishedAt.IsZero() || task.Size != 1024 {
		t.Fatalf("completed task missing finish stamp/size: %+v", task)
	}
	taskStore.Add(users.Task{GID: "g9", UserID: 100, Link: "http://x", Engine: "aria2", Status: "completed"})
	taskStore.Add(users.Task{GID: "yt9", UserID: 200, Link: "http://y", Engine: "ytdlp", Status: "failed"})

	h := formatTelegramHistory(100)
	if !strings.Contains(h, "show.mkv") || strings.Contains(h, "http://x") || strings.Contains(h, "http://y") {
		t.Fatalf("user history wrong:\n%s", h)
	}
	h = formatTelegramHistory(1)
	if !strings.Contains(h, "show.mkv") || !strings.Contains(h, "http://y") {
		t.Fatalf("admin history should include all non-aria2:\n%s", h)
	}

	// cancelling a live fetch forgets it
	registerLocalFetch("tg9", "show.mkv", 100, 100, 1024, nil, nil)
	if !cancelTelegramFetch("tg9") {
		t.Fatal("expected live cancel to report true")
	}
	if _, ok := taskStore.Get("tg9"); ok {
		t.Fatal("cancelled task record should be gone")
	}
	if cancelTelegramFetch("tg9") {
		t.Fatal("second cancel should report false")
	}
	// forgetting a history entry
	taskStore.Remove("yt9")
	if _, ok := taskStore.Get("yt9"); ok {
		t.Fatal("removed task should be gone")
	}
}
