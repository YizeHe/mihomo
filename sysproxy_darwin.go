//go:build darwin

package main

// macOS: no system proxy control via registry (use networksetup instead)
func SetSystemProxy() error   { return nil }
func UnsetSystemProxy() error { return nil }
