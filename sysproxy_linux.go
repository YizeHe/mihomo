//go:build linux

package main

// Linux: no system proxy control
func SetSystemProxy() error   { return nil }
func UnsetSystemProxy() error { return nil }
