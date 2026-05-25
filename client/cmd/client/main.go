package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"chatview/client/internal/config"
	"chatview/client/internal/core"
	"chatview/client/internal/identity"
	"chatview/client/internal/platform"
	"chatview/client/internal/rpcclient"
	"chatview/client/internal/storage"
	chatui "chatview/client/internal/ui"
)

func main() {
	installLogFilter()
	warnIfLikelyBlurry()

	var configPath string
	var dataDir string
	var target string
	tlsFlag := optionalBool{}

	flag.StringVar(&configPath, "config", "", "YAML config path")
	flag.StringVar(&dataDir, "data-dir", "", "client data directory")
	flag.StringVar(&target, "target", "", "gRPC target")
	flag.Var(&tlsFlag, "tls", "override gRPC TLS: true or false")
	flag.Parse()

	options, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if dataDir != "" {
		options.DataDir = dataDir
	}
	if target != "" {
		options.GRPCTarget = target
	}
	if tlsFlag.set {
		options.GRPCUseTLS = tlsFlag.value
	}

	if err := os.MkdirAll(options.DataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	identityStore := identity.NewStore(options.IdentityPath())
	rpc, err := rpcclient.New(rpcclient.Options{
		Target:                options.GRPCTarget,
		UseTLS:                options.GRPCUseTLS,
		CACertPath:            options.GRPCCACertPath,
		SSLTargetNameOverride: options.GRPCSSLTargetNameOverride,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer rpc.Close()

	cache, err := storage.Open(options.CachePath())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cache.Close()

	service := core.NewService(identityStore, rpc, cache)
	chatui.Run(service)
}

func installLogFilter() {
	log.SetOutput(filterWriter{next: os.Stderr})
}

type filterWriter struct {
	next *os.File
}

func (w filterWriter) Write(p []byte) (int, error) {
	message := string(p)
	if strings.Contains(message, "PlatformError: Wayland: Focusing a window requires user interaction") {
		return len(p), nil
	}
	return w.next.Write(p)
}

func warnIfLikelyBlurry() {
	if os.Getenv("XDG_SESSION_TYPE") != "wayland" || platform.NativeWayland {
		return
	}
	fmt.Fprintln(os.Stderr, "warning: running on Wayland without native Wayland build tag; use `go run -tags wayland ./cmd/client` if the window looks blurry")
}

type optionalBool struct {
	set   bool
	value bool
}

func (b *optionalBool) Set(value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	b.set = true
	b.value = parsed
	return nil
}

func (b *optionalBool) String() string {
	if !b.set {
		return ""
	}
	return strconv.FormatBool(b.value)
}
