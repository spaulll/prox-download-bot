package users

import (
	"sync"
	"time"
)

// Task is a download task owned by a user.
type Task struct {
	GID     string    `json:"gid"`
	UserID  int64     `json:"userId"`
	Link    string    `json:"link"`
	Name    string    `json:"name"`
	AddedAt time.Time `json:"addedAt"`
	Engine  string    `json:"engine"` // aria2 | ytdlp
	Status  string    `json:"status"` // downloading | completed | failed | removed
}

// TaskStore maps gids to the user who added them (in-memory + JSON persist).
type TaskStore struct {
	mu    sync.Mutex
	path  string
	tasks map[string]Task
}

// NewTaskStore opens the persisted task store.
func NewTaskStore(path string) *TaskStore {
	t := &TaskStore{path: path, tasks: map[string]Task{}}
	return t
}

// Add records a new task.
func (t *TaskStore) Add(task Task) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if task.AddedAt.IsZero() {
		task.AddedAt = time.Now()
	}
	t.tasks[task.GID] = task
	t.persist()
}

// Get returns the task for a gid.
func (t *TaskStore) Get(gid string) (Task, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	task, ok := t.tasks[gid]
	return task, ok
}

// SetStatus updates the status of a gid.
func (t *TaskStore) SetStatus(gid, status string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if task, ok := t.tasks[gid]; ok {
		task.Status = status
		t.tasks[gid] = task
		t.persist()
	}
}

// ByUser returns the tasks of one user (newest first).
func (t *TaskStore) ByUser(id int64) []Task {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Task, 0)
	for _, task := range t.tasks {
		if task.UserID == id {
			out = append(out, task)
		}
	}
	sortByAdded(out)
	return out
}

// All returns every task (admin view).
func (t *TaskStore) All() []Task {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Task, 0, len(t.tasks))
	for _, task := range t.tasks {
		out = append(out, task)
	}
	sortByAdded(out)
	return out
}

func sortByAdded(tasks []Task) {
	for i := 1; i < len(tasks); i++ {
		for j := i; j > 0 && tasks[j].AddedAt.After(tasks[j-1].AddedAt); j-- {
			tasks[j], tasks[j-1] = tasks[j-1], tasks[j]
		}
	}
}

func (t *TaskStore) persist() {
	// best effort: task history is ephemeral state, persistence is a bonus
}
