package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/log"

	"go.uber.org/automaxprocs/maxprocs"
)

//go:embed config.yaml
var embeddedConfig []byte

const (
	BuiltInVersion = 1
	UpdateURL      = "https://vpn.tangentlab.cn"
	SupportEmail   = "tangent2533@gmail.com"
	VersionURL     = "http://8.154.28.102/mihomo_ver.txt"
)

// BackendURL is overridden at build time via -ldflags
var BackendURL = "http://47.84.235.137:7878"

var (
	version    bool
	homeDir    string
	NoSysProxy bool // -n flag: don't set system proxy
)

func init() {
	flag.StringVar(&homeDir, "d", "", "set configuration directory")
	flag.BoolVar(&version, "v", false, "show current version of TangentVPN")
	flag.BoolVar(&NoSysProxy, "n", false, "start proxy without setting Windows system proxy")
	flag.Parse()
}

func main() {
	_, _ = maxprocs.Set(maxprocs.Logger(func(string, ...any) {}))

	if version {
		fmt.Printf("TangentVPN %s %s %s with %s\n",
			C.Version, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return
	}

	// Set home directory for mihomo runtime data
	if homeDir == "" {
		exe, err := os.Executable()
		if err != nil {
			homeDir, _ = os.Getwd()
		} else {
			homeDir = filepath.Dir(exe)
		}
	}
	C.SetHomeDir(homeDir)

	// Version check (blocking — must succeed to proceed)
	if err := checkVersion(); err != nil {
		fmt.Println("╔══════════════════════════════════════════════════╗")
		fmt.Println("║           版本检查失败，无法启动                   ║")
		fmt.Println("╠══════════════════════════════════════════════════╣")
		fmt.Printf("║  %-48s ║\n", err.Error())
		fmt.Println("║                                                  ║")
		fmt.Printf("║  请前往 %-40s ║\n", UpdateURL)
		fmt.Printf("║  或联系 %-40s ║\n", SupportEmail)
		fmt.Println("╚══════════════════════════════════════════════════╝")
		fmt.Println("\n按回车键退出...")
		fmt.Scanln()
		os.Exit(1)
	}

	log.Infoln("TangentVPN v%s starting...", C.Version)

	// Launch native GUI (blocks until window is closed)
	runGUI()

	// Cleanup
	executor.Shutdown()
}
