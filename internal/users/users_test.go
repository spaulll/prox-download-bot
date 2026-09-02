package users

import "testing"

func TestStoreApprovalFlow(t *testing.T) {
	path := t.TempDir() + "/users.json"
	s := Open(path)

	// new user -> pending
	role := s.UpsertStarted(100, "alice", "Alice A")
	if role != RolePending {
		t.Errorf("new user role = %v, want pending", RoleName(role))
	}
	if s.Approved(100) {
		t.Error("pending user approved?")
	}

	// approve
	s.SetRole(100, RoleApproved)
	if !s.Approved(100) {
		t.Error("approved user not approved")
	}

	// re-start keeps approved
	if role := s.UpsertStarted(100, "alice", "Alice A"); role != RoleApproved {
		t.Errorf("role after re-start = %v, want approved", RoleName(role))
	}

	// deny
	s.SetRole(101, RoleDenied)
	if s.Approved(101) {
		t.Error("denied user approved?")
	}

	// persistence across reopen
	s2 := Open(path)
	if !s2.Approved(100) {
		t.Error("approval not persisted")
	}
}

func TestTaskStore(t *testing.T) {
	ts := NewTaskStore(t.TempDir() + "/tasks.json")
	ts.Add(Task{GID: "g1", UserID: 100, Link: "http://x", Engine: "aria2"})
	ts.Add(Task{GID: "g2", UserID: 200, Link: "http://y", Engine: "ytdlp"})
	ts.Add(Task{GID: "g3", UserID: 100, Link: "http://z", Engine: "aria2"})

	if len(ts.ByUser(100)) != 2 {
		t.Errorf("ByUser(100) = %d tasks, want 2", len(ts.ByUser(100)))
	}
	if len(ts.All()) != 3 {
		t.Errorf("All() = %d tasks, want 3", len(ts.All()))
	}
	ts.SetStatus("g1", "completed")
	if task, _ := ts.Get("g1"); task.Status != "completed" {
		t.Errorf("status = %q", task.Status)
	}
}
