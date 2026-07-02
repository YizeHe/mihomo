package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func main() {
	fmt.Println("=== Proxy Test ===")

	// Test 1: Direct connection (no proxy)
	fmt.Println("\n[1] Direct connection (no proxy):")
	ip := getIP(nil)
	if ip != "" {
		fmt.Printf("  IP: %s\n", ip)
	} else {
		fmt.Println("  Failed to get IP")
	}

	// Test 2: Through proxy
	fmt.Println("\n[2] Through proxy (127.0.0.1:7899):")
	proxyURL, _ := url.Parse("http://127.0.0.1:7899")
	ip = getIP(&http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	})
	if ip != "" {
		fmt.Printf("  IP: %s\n", ip)
		fmt.Println("\n  => Proxy is working!")
	} else {
		fmt.Println("  Failed to connect through proxy")
		fmt.Println("  => Make sure TangentVPN is running with -n flag")
	}
}

func getIP(transport *http.Transport) string {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	if transport != nil {
		client.Transport = transport
	}

	resp, err := client.Get("https://api.ipify.org?format=json")
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("  Read error: %v\n", err)
		return ""
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("  Parse error: %v\n", err)
		return ""
	}

	ip, _ := result["ip"].(string)
	return ip
}
