package main

import (
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

// User represents a VPN user stored in bbolt.
type User struct {
	Email     string    `json:"email"`
	Password  string    `json:"password"` // bcrypt hash
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IsActive  bool      `json:"is_active"`
}

var db *bolt.DB

// InitDB opens (or creates) the bbolt database at the given path.
func InitDB(path string) error {
	var err error
	db, err = bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return err
	}
	// Ensure the "users" bucket exists.
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("users"))
		return err
	})
}

// CreateUser stores a new user. Returns an error if the email already exists.
func CreateUser(user *User) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		if b.Get([]byte(user.Email)) != nil {
			return errors.New("user already exists")
		}
		data, err := json.Marshal(user)
		if err != nil {
			return err
		}
		return b.Put([]byte(user.Email), data)
	})
}

// GetUser retrieves a user by email. Returns nil if not found.
func GetUser(email string) (*User, error) {
	var user *User
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		v := b.Get([]byte(email))
		if v == nil {
			return errors.New("user not found")
		}
		user = &User{}
		return json.Unmarshal(v, user)
	})
	return user, err
}

// UpdateUser overwrites the user record for the given email.
func UpdateUser(email string, user *User) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		if b.Get([]byte(email)) == nil {
			return errors.New("user not found")
		}
		data, err := json.Marshal(user)
		if err != nil {
			return err
		}
		return b.Put([]byte(email), data)
	})
}

// DeleteUser removes the user record for the given email.
func DeleteUser(email string) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		if b.Get([]byte(email)) == nil {
			return errors.New("user not found")
		}
		return b.Delete([]byte(email))
	})
}

// ListUsers returns all users in the database.
func ListUsers() ([]User, error) {
	var users []User
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		return b.ForEach(func(k, v []byte) error {
			var u User
			if err := json.Unmarshal(v, &u); err != nil {
				return err
			}
			users = append(users, u)
			return nil
		})
	})
	return users, err
}
