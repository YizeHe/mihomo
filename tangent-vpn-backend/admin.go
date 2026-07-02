package main

import (
	"fmt"
	"time"
)

// AdminPassword computes today's admin password: (month+day)%10 + "TangentLab2533434893qq"
func AdminPassword() string {
	now := time.Now()
	digit := (int(now.Month()) + now.Day()) % 10
	return fmt.Sprintf("%dTangentLab2533434893qq", digit)
}

// ValidateAdmin checks whether the supplied password matches today's admin password.
func ValidateAdmin(adminPassword string) bool {
	return adminPassword == AdminPassword()
}
