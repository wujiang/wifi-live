package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func wifiIface() string {
	out, _ := run("networksetup", "-listallhardwareports")
	if m := regexp.MustCompile(`Hardware Port: Wi-Fi\s*\nDevice: (\S+)`).FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return "en0"
}

func macInt(mac string) (uint64, bool) {
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return 0, false
	}
	var v uint64
	for _, p := range parts {
		if len(p) == 0 || len(p) > 2 {
			return 0, false
		}
		b, err := strconv.ParseUint(p, 16, 8)
		if err != nil {
			return 0, false
		}
		v = v<<8 | b
	}
	return v, true
}

// unitMACs returns the MAC of each mesh unit, from the ARP table (pinging first so it's populated).
func unitMACs() map[string]uint64 {
	macs := map[string]uint64{}
	for _, n := range UnitNames {
		if m, ok := macInt(KnownMACs[n]); ok {
			macs[n] = m
		}
	}
	re := regexp.MustCompile(` at ([0-9a-f:]+)`)
	for _, name := range UnitNames {
		ip := Units[name]
		run("ping", "-c", "1", "-W", "500", ip)
		out, _ := run("arp", "-n", ip)
		if m := re.FindStringSubmatch(out); m != nil {
			if v, ok := macInt(m[1]); ok {
				macs[name] = v
			}
		}
	}
	return macs
}

// unitFor: each unit's Wi-Fi BSSIDs sit a few addresses above its wired MAC, sometimes with
// the locally-administered bit set. Match on the first 5 bytes and a nearby last byte.
func unitFor(bssid string) string {
	b, ok := macInt(bssid)
	if !ok {
		return ""
	}
	best, bestD := "", -1
	for _, name := range UnitNames {
		m, ok := MACs[name]
		if !ok {
			continue
		}
		for _, cand := range []uint64{b, b ^ (0x02 << 40)} {
			d := int(cand&0xFF) - int(m&0xFF)
			if d < 0 {
				d = -d
			}
			if cand>>8 == m>>8 && d <= 16 && (bestD < 0 || d < bestD) {
				best, bestD = name, d
			}
		}
	}
	return best
}

// startHelper keeps WifiInfo.app running; it writes the link (with BSSID) to NowFile.
func startHelper() {
	touch(WantFile)
	if st, err := os.Stat(HelperApp); err == nil && st.IsDir() {
		if exec.Command("pgrep", "-x", "WifiInfo").Run() != nil {
			exec.Command("open", "-g", HelperApp).Run()
		}
	}
}

type Link struct {
	RSSI, Noise, Rate, Ch int
	BSSID, SSID, Unit     string
	HasAuth               bool // helper reported a Location authorization status
}

func num(d map[string]any, k string) (float64, bool) {
	v, ok := d[k].(float64)
	return v, ok
}

func str(d map[string]any, k string) string {
	s, _ := d[k].(string)
	return s
}

// helperLink reads the link from WifiInfo.app. fresh is false when the file is missing or
// stale; a nil link with fresh=true means the helper says we're not connected.
func helperLink() (link *Link, fresh bool) {
	data, err := os.ReadFile(NowFile)
	if err != nil {
		return nil, false
	}
	var d map[string]any
	if json.Unmarshal(data, &d) != nil {
		return nil, false
	}
	t, ok := num(d, "t")
	if !ok || now()-t > 3 {
		return nil, false
	}
	rssi, ok := num(d, "rssi")
	if !ok {
		return nil, false
	}
	if rssi == 0 {
		return nil, true
	}
	noise, ok1 := num(d, "noise")
	rate, ok2 := num(d, "rate")
	ch, ok3 := num(d, "ch")
	if !ok1 || !ok2 || !ok3 {
		return nil, false
	}
	return &Link{RSSI: int(rssi), Noise: int(noise), Rate: int(rate), Ch: int(ch),
		BSSID: str(d, "bssid"), SSID: str(d, "ssid"), HasAuth: d["auth"] != nil}, true
}

type Scan struct {
	Age   float64
	Units map[string]map[string]int // unit -> band ("5" or "2.4") -> best rssi
}

// unitScan returns the best signal per unit and band from the latest scan, for one network
// name ("" = any), or nil.
func unitScan(ssid string) *Scan {
	data, err := os.ReadFile(ScanFile)
	if err != nil {
		return nil
	}
	var d struct {
		T    *float64 `json:"t"`
		Nets []struct {
			SSID  string   `json:"ssid"`
			BSSID string   `json:"bssid"`
			Band  string   `json:"band"`
			RSSI  *float64 `json:"rssi"`
		} `json:"nets"`
	}
	if json.Unmarshal(data, &d) != nil || d.T == nil {
		return nil
	}
	units := map[string]map[string]int{}
	for _, n := range d.Nets {
		if ssid != "" && n.SSID != ssid {
			continue
		}
		u := unitFor(n.BSSID)
		if u == "" || n.RSSI == nil {
			continue
		}
		b := "5"
		if n.Band == "2.4" {
			b = "2.4"
		}
		cur := units[u]
		if cur == nil {
			cur = map[string]int{}
			units[u] = cur
		}
		if old, ok := cur[b]; !ok || int(*n.RSSI) > old {
			cur[b] = int(*n.RSSI)
		}
	}
	return &Scan{Age: now() - *d.T, Units: units}
}

func requestScan() float64 {
	touch(ScanReq)
	return now()
}

func wifiLink() *Link {
	if l, fresh := helperLink(); fresh {
		return l
	}
	return coreWLANLink()
}

var pingRe = regexp.MustCompile(`time=([\d.]+) ms`)

func ping(host string) *float64 {
	out, _ := runTimeout(3*time.Second, "ping", "-c", "1", "-W", "1000", "-b", Iface, host)
	m := pingRe.FindStringSubmatch(out)
	if m == nil {
		return nil
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	return &v
}

func quality(rssi *int) (string, int) {
	switch {
	case rssi == nil:
		return "NOT CONNECTED", 4
	case *rssi >= -50:
		return "Excellent", 1
	case *rssi >= -60:
		return "Very good", 1
	case *rssi >= -67:
		return "Good", 2
	case *rssi >= -75:
		return "Fair: slow, may stutter", 3
	}
	return "Poor: expect drops", 4
}

func band(ch int) string {
	switch {
	case ch == 0:
		return "?"
	case ch <= 14:
		return "2.4 GHz"
	}
	return "5 GHz"
}
