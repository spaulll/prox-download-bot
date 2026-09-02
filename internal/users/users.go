// Package users implements multi-user management with admin approval.
//
// Copyright 2026 spaulll - prox-download-bot (Apache-2.0)
package users

import (
	"encoding/json"
	"os"
	"sync"
)

// Role of a user.
type Role int

const (
	// RoleAdmin is the configured owner (config user-id).
	RoleAdmin Role = iota
	// RoleApproved can use the bot.
	RoleApproved
	// RolePending has requested access.
	RolePending
	// RoleDenied was refused.
	RoleDenied
)

// User is a stored bot user.
type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FullName  string `json:"fullName"`
	Role      Role   `json:"role"`
	StartedAt int64  `json:"startedAt"`
}

// Store persists users in a JSON file.
type Store struct {
	mu   sync.Mutex
	path string
	data struct {
		Users []User `json:"users"`
	}
}

// Open loads (or initializes) the store at path.
func Open(path string) *Store {
	s := &Store{path: path}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s.data)
	}
	return s
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

// Get returns the user and whether they exist.
func (s *Store) Get(id int64) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.data.Users {
		if u.ID == id {
			return u, true
		}
	}
	return User{}, false
}

// UpsertStarted registers a /start from a user. Returns their role *after*
// the call: admins stay admin; known approved users stay approved; new users
// become pending.
func (s *Store) UpsertStarted(id int64, username, fullName string) Role {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.data.Users {
		if u.ID == id {
			u.Username = username
			u.FullName = fullName
			s.data.Users[i] = u
			_ = s.save()
			return u.Role
		}
	}
	s.data.Users = append(s.data.Users, User{
		ID: id, Username: username, FullName: fullName, Role: RolePending,
	})
	_ = s.save()
	return RolePending
}

// SetRole updates a user's role.
func (s *Store) SetRole(id int64, role Role) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.data.Users {
		if u.ID == id {
			u.Role = role
			s.data.Users[i] = u
			_ = s.save()
			return
		}
	}
	s.data.Users = append(s.data.Users, User{ID: id, Role: role})
	_ = s.save()
}

// Approved reports whether the id may use the bot.
func (s *Store) Approved(id int64) bool {
	u, ok := s.Get(id)
	return ok && (u.Role == RoleApproved || u.Role == RoleAdmin)
}

// Pending returns all pending users.
func (s *Store) Pending() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, 0)
	for _, u := range s.data.Users {
		if u.Role == RolePending {
			out = append(out, u)
		}
	}
	return out
}

// All returns every stored user (admin views).
func (s *Store) All() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, len(s.data.Users))
	copy(out, s.data.Users)
	return out
}

// RoleName renders a role for messages.
func RoleName(r Role) string {
	switch r {
	case RoleAdmin:
		return "admin"
	case RoleApproved:
		return "approved"
	case RolePending:
		return "pending"
	case RoleDenied:
		return "denied"
	default:
		return "unknown"
	}
}
