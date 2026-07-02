package main

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	proxyAddr           = "127.0.0.1:7899"
	proxyOverride       = "localhost;127.0.0.1;<local>"
)

// SetSystemProxy enables the Windows system proxy pointing to mihomo
func SetSystemProxy() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry key: %w", err)
	}
	defer k.Close()

	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("set ProxyEnable: %w", err)
	}
	if err := k.SetStringValue("ProxyServer", proxyAddr); err != nil {
		return fmt.Errorf("set ProxyServer: %w", err)
	}
	if err := k.SetStringValue("ProxyOverride", proxyOverride); err != nil {
		return fmt.Errorf("set ProxyOverride: %w", err)
	}
	return nil
}

// UnsetSystemProxy disables the Windows system proxy
func UnsetSystemProxy() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry key: %w", err)
	}
	defer k.Close()

	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return fmt.Errorf("set ProxyEnable: %w", err)
	}
	return nil
}

// GetSystemProxyStatus returns whether the system proxy is enabled
func GetSystemProxyStatus() (bool, string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.READ)
	if err != nil {
		return false, "", fmt.Errorf("open registry key: %w", err)
	}
	defer k.Close()

	enabled, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil {
		return false, "", nil // key might not exist, treat as disabled
	}

	server, _, _ := k.GetStringValue("ProxyServer")
	return enabled != 0, server, nil
}
