package telegram

import (
	"testing"

	"DownloadBot/internal/users"
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
