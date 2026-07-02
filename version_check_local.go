//go:build localtest

package main

// checkVersion is skipped in localtest builds
func checkVersion() error {
	return nil
}
