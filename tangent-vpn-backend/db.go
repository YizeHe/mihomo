package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// User represents a VPN user stored in bbolt.
type User struct {
	Email      string    `json:"email"`
	Password   string    `json:"password"` // bcrypt hash
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	IsActive   bool      `json:"is_active"`
	InviteCode string    `json:"invite_code,omitempty"` // distributor code used at registration
}

// Distributor represents a reseller agent.
type Distributor struct {
	Code           string    `json:"code"`
	CommissionRate float64   `json:"commission_rate"` // earnings per sale
	CreatedAt      time.Time `json:"created_at"`
}

// Sale records a commission event when admin adds days to a referred user.
type Sale struct {
	DistributorCode string    `json:"distributor_code"`
	UserEmail       string    `json:"user_email"`
	Amount          float64   `json:"amount"`
	CreatedAt       time.Time `json:"created_at"`
}

// ActivationCode represents a card/key for activation.
type ActivationCode struct {
	Code      string    `json:"code"`
	Type      string    `json:"type"` // "year" or "month"
	CreatedAt time.Time `json:"created_at"`
}

var db *bolt.DB

// InitDB opens (or creates) the bbolt database at the given path.
func InitDB(path string) error {
	var err error
	db, err = bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range []string{"users", "distributors", "sales", "activation_codes"} {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return err
			}
		}
		return nil
	})
}

// ---------- User ----------

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

func DeleteUser(email string) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		if b.Get([]byte(email)) == nil {
			return errors.New("user not found")
		}
		return b.Delete([]byte(email))
	})
}

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

// ---------- Distributor ----------

func CreateDistributor(d *Distributor) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("distributors"))
		if b.Get([]byte(d.Code)) != nil {
			return errors.New("distributor code already exists")
		}
		data, err := json.Marshal(d)
		if err != nil {
			return err
		}
		return b.Put([]byte(d.Code), data)
	})
}

func GetDistributor(code string) (*Distributor, error) {
	var d *Distributor
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("distributors"))
		v := b.Get([]byte(code))
		if v == nil {
			return errors.New("distributor not found")
		}
		d = &Distributor{}
		return json.Unmarshal(v, d)
	})
	return d, err
}

func ListDistributors() ([]Distributor, error) {
	var list []Distributor
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("distributors"))
		return b.ForEach(func(k, v []byte) error {
			var d Distributor
			if err := json.Unmarshal(v, &d); err != nil {
				return err
			}
			list = append(list, d)
			return nil
		})
	})
	return list, err
}

func DeleteDistributor(code string) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("distributors"))
		if b.Get([]byte(code)) == nil {
			return errors.New("distributor not found")
		}
		return b.Delete([]byte(code))
	})
}

// ---------- Sale ----------

func RecordSale(distributorCode, userEmail string, amount float64) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("sales"))
		key := []byte(fmt.Sprintf("%s|%s", distributorCode, userEmail))
		if b.Get(key) != nil {
			return nil // already recorded
		}
		sale := Sale{
			DistributorCode: distributorCode,
			UserEmail:       userEmail,
			Amount:          amount,
			CreatedAt:       time.Now(),
		}
		data, err := json.Marshal(sale)
		if err != nil {
			return err
		}
		return b.Put(key, data)
	})
}

func ListSales() ([]Sale, error) {
	var list []Sale
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("sales"))
		return b.ForEach(func(k, v []byte) error {
			var s Sale
			if err := json.Unmarshal(v, &s); err != nil {
				return err
			}
			list = append(list, s)
			return nil
		})
	})
	return list, err
}

func GetSalesByDistributor(code string) ([]Sale, error) {
	var list []Sale
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("sales"))
		return b.ForEach(func(k, v []byte) error {
			var s Sale
			if err := json.Unmarshal(v, &s); err != nil {
				return err
			}
			if s.DistributorCode == code {
				list = append(list, s)
			}
			return nil
		})
	})
	return list, err
}

// ---------- Activation Code ----------

func AddActivationCode(code *ActivationCode) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("activation_codes"))
		if b.Get([]byte(code.Code)) != nil {
			return nil // already exists, skip
		}
		data, err := json.Marshal(code)
		if err != nil {
			return err
		}
		return b.Put([]byte(code.Code), data)
	})
}

func GetActivationCode(code string) (*ActivationCode, error) {
	var ac *ActivationCode
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("activation_codes"))
		v := b.Get([]byte(code))
		if v == nil {
			return errors.New("activation code not found")
		}
		ac = &ActivationCode{}
		return json.Unmarshal(v, ac)
	})
	return ac, err
}

func DeleteActivationCode(code string) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("activation_codes"))
		if b.Get([]byte(code)) == nil {
			return errors.New("activation code not found")
		}
		return b.Delete([]byte(code))
	})
}

func ListActivationCodes() ([]ActivationCode, error) {
	var list []ActivationCode
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("activation_codes"))
		return b.ForEach(func(k, v []byte) error {
			var ac ActivationCode
			if err := json.Unmarshal(v, &ac); err != nil {
				return err
			}
			list = append(list, ac)
			return nil
		})
	})
	return list, err
}
