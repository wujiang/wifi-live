// Live Wi-Fi signal meter for walking a Mac around the house.
//
// Shows this Mac's Wi-Fi signal, speed, channel and which unit (router or access point)
// it is connected to, once a second, plus live pings to the router and the internet over
// Wi-Fi (even if a cable is plugged in).
//
// Type a room name and press Enter to save that spot to wifi_survey.csv
// (average of the last 10 seconds). Quit with Esc or Ctrl+C.
//
// Build with `make`. Signal, speed and channel come from CoreWLAN directly. Showing the
// connected unit and the scan of all units also needs WifiInfo.app (built from
// wifiinfo_app.swift, kept next to the binary) with Location permission, since macOS
// hides access point addresses (BSSIDs) from anything else.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gdamore/tcell/v2"
)

var (
	Here       = hereDir()
	ConfigFile = filepath.Join(Here, "wifi_live.json")
	Survey     = filepath.Join(Here, "wifi_survey.csv")
	HelperApp  = filepath.Join(Here, "WifiInfo.app")
	NowFile    = filepath.Join(Here, ".wifi_now.json")
	ScanFile   = filepath.Join(Here, ".wifi_scan.json") // WifiInfo.app rescans every 20 s
	ScanReq    = filepath.Join(Here, ".wifi_scan_req")  // touch to ask WifiInfo.app for a scan now
	WantFile   = filepath.Join(Here, ".wifi_want")
	Iface      = wifiIface()
	MACs       = map[string]uint64{}
)

// hereDir is the binary's folder, or the current folder when that has no WifiInfo.app (go run).
func hereDir() string {
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			dir := filepath.Dir(exe)
			if _, err := os.Stat(filepath.Join(dir, "WifiInfo.app")); err == nil {
				return dir
			}
		}
	}
	dir, _ := os.Getwd()
	return dir
}

func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

func runTimeout(d time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

func touch(path string) {
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		f.Close()
		t := time.Now()
		os.Chtimes(path, t, t)
	}
}

func main() {
	if err := loadConfig(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	startHelper()
	MACs = unitMACs()

	scr, err := tcell.NewScreen()
	if err == nil {
		err = scr.Init()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	scr.HideCursor()
	mainLoop(scr)
	scr.Fini()
	if _, err := os.Stat(Survey); err == nil {
		fmt.Printf("Saved spots are in %s\n", Survey)
	}
}
