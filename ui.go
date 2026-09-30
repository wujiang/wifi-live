package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

const Spark = "▁▂▃▄▅▆▇█"

var (
	plain = tcell.StyleDefault
	bold  = plain.Bold(true)
	dim   = plain.Dim(true)
)

// C mirrors the curses color pairs: 1 green, 2 cyan, 3 yellow, 4 red.
func C(n int) tcell.Style {
	return plain.Foreground([]tcell.Color{tcell.ColorDefault, tcell.ColorGreen, tcell.ColorTeal,
		tcell.ColorOlive, tcell.ColorMaroon}[n])
}

type Pending struct {
	Spot
	RequestedAt float64
}

type UI struct {
	scr  tcell.Screen
	W, H int
}

// put writes s at (y, x), clipped like curses addnstr to the screen's width.
func (u *UI) put(y, x int, s string, st tcell.Style) {
	if y >= u.H {
		return
	}
	limit := u.W - 1
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if x+w > limit {
			return
		}
		u.scr.SetContent(x, y, r, nil, st)
		x += w
	}
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

func sparkChar(frac float64) string {
	i := int(math.Min(7, math.Max(0, frac*7)))
	return string([]rune(Spark)[i])
}

func draw(u *UI, sm *Sampler, typed string, saved []Spot, pending *Pending) {
	u.scr.Clear()
	u.W, u.H = u.scr.Size()
	W, H := u.W, u.H
	put := u.put
	hist := sm.Window(120)
	var last Sample
	if len(hist) > 0 {
		last = hist[len(hist)-1]
	}
	link := last.Link
	var rssi *int
	if link != nil {
		rssi = &link.RSSI
	}
	label, color := "Measuring…", 2
	if len(hist) > 0 {
		label, color = quality(rssi)
	}

	put(0, 2, "Wi-Fi live meter", bold)
	put(0, 20, fmt.Sprintf("· %s · %s %s · %s", Iface, RouterName, Router, time.Now().Format("15:04:05")), dim)

	put(2, 2, "Signal", dim)
	put(3, 2, fmt.Sprintf("%4s dBm", optInt(rssi, "--")), C(color).Bold(true))
	put(3, 14, label, C(color))
	// bar from -90 (empty) to -30 (full)
	bw := max(10, min(50, W-6))
	fill := 0
	if rssi != nil {
		fill = int(float64(bw) * math.Min(1, math.Max(0, float64(*rssi+90)/60)))
	}
	put(4, 2, strings.Repeat("█", fill), C(color))
	put(4, 2+fill, strings.Repeat("░", bw-fill), dim)
	put(5, 2, fmt.Sprintf("%-*s-30", bw-3, "-90"), dim)

	if link != nil {
		snr := link.RSSI - link.Noise
		put(7, 2, fmt.Sprintf("Speed %4d Mbps   %s ch %-4d noise %d dBm   SNR %d dB",
			link.Rate, band(link.Ch), link.Ch, link.Noise, snr), plain)
		switch {
		case link.Unit != "":
			put(8, 2, "Connected to ", dim)
			put(8, 15, link.Unit, bold)
			put(8, 16+len([]rune(link.Unit)), "("+link.BSSID+")", dim)
		case link.HasAuth:
			put(8, 2, "Connected to: unknown (allow Location for WifiInfo in System Settings)", C(3))
		default:
			put(8, 2, "Connected to: unknown (WifiInfo.app not running)", C(3))
		}
	} else if len(hist) > 0 {
		put(7, 2, "Not connected to Wi-Fi", C(4))
	}

	r, n := last.Router, last.Net
	s30 := stats(sm.Window(30))
	put(10, 2, RouterName, dim)
	if r != nil {
		put(10, 17, fmt.Sprintf("%6.1f ms", *r), C(map[bool]int{true: 1, false: 3}[*r < 50]))
	} else {
		put(10, 17, "   no reply", C(4))
	}
	lossColor := 1
	if s30.Loss != nil && *s30.Loss != 0 {
		lossColor = 4
		if *s30.Loss < 10 {
			lossColor = 3
		}
	}
	put(10, 30, fmt.Sprintf("loss %s%% last 30s", optInt(s30.Loss, "--")), C(lossColor))
	put(11, 2, "Internet", dim)
	if n != nil {
		put(11, 17, fmt.Sprintf("%6.0f ms", *n), C(map[bool]int{true: 1, false: 3}[*n < 80]))
	} else {
		put(11, 17, "   no reply", C(4))
	}

	// 2-minute sparklines of signal (-90..-30) and router ping (0..30 ms)
	recent := hist[len(hist)-min(len(hist), max(0, W-4)):]
	var sp, sp2 strings.Builder
	for _, h := range recent {
		if h.Link != nil {
			sp.WriteString(sparkChar(float64(h.Link.RSSI+90) / 60))
		} else {
			sp.WriteString(" ")
		}
		if h.Router == nil {
			sp2.WriteString("·")
		} else {
			sp2.WriteString(sparkChar(*h.Router / 30))
		}
	}
	put(13, 2, "Signal, last 2 min", dim)
	put(14, 2, sp.String(), C(color))
	put(15, 2, RouterName+" ping, last 2 min  (· = no reply)", dim)
	put(16, 2, sp2.String(), plain)
	if ro := sm.LastRoam(); ro != nil {
		t := time.Unix(0, int64(ro.T*1e9))
		put(17, 2, fmt.Sprintf("Last switch %s: %s -> %s at %d dBm", t.Format("15:04:05"), ro.From, ro.To, ro.RSSI), dim)
	}

	// every unit's signal from the latest background scan
	y := 19
	ssid := ""
	if link != nil {
		ssid = link.SSID
	}
	if sc := unitScan(ssid); sc != nil {
		put(y, 2, fmt.Sprintf("All units from here (scan %.0fs ago)          5 GHz    2.4 GHz", sc.Age), dim)
		for i, name := range UnitNames {
			v := sc.Units[name]
			here := link != nil && link.Unit == name
			mark, st := "  ", plain
			if here {
				mark, st = "● ", bold
			}
			put(y+1+i, 2, fmt.Sprintf("%s%-16s", mark, name), st)
			for _, c := range []struct {
				x int
				b string
			}{{44, "5"}, {53, "2.4"}} {
				if x, ok := v[c.b]; ok {
					_, q := quality(&x)
					put(y+1+i, c.x, fmt.Sprintf("%4d dBm", x), C(q))
				} else {
					put(y+1+i, c.x, "   --   ", dim)
				}
			}
		}
		y += len(UnitNames) + 2
	}

	put(y, 2, "Type a room name + Enter to save this spot (10 s average). Esc to quit.", dim)
	put(y+1, 2, "Room: "+typed+"▏", bold)
	if pending != nil {
		put(y+2, 2, fmt.Sprintf("Saving '%s': scanning all units…", pending.Name), C(2))
	}
	if len(saved) > 0 {
		put(y+3, 2, "Saved spots"+strings.Repeat(" ", 88)+"all units, 5 GHz", dim)
		shown := saved[len(saved)-min(len(saved), max(1, H-y-5)):]
		for i, s := range shown {
			_, c := quality(s.St.RSSI)
			unit := s.Unit
			if unit == "" {
				unit = "?"
			}
			rate := "--"
			if s.St.Rate != nil && *s.St.Rate != 0 {
				rate = strconv.Itoa(*s.St.Rate)
			}
			put(y+4+i, 2, fmt.Sprintf("%-16s %-13s %4s dBm  %4s Mbps  %-7s loss %s%%  ping %s ms",
				truncRunes(s.Name, 16), truncRunes(unit, 13), optInt(s.St.RSSI, "--"), rate, band(s.Ch),
				optInt(s.St.Loss, "--"), optFloat(s.St.Ping, "--")), C(c))
			var parts []string
			for _, un := range UnitNames {
				v := "--"
				if s.Scan != nil {
					if x, ok := s.Scan.Units[un]["5"]; ok {
						v = strconv.Itoa(x)
					}
				}
				parts = append(parts, strings.Fields(un)[0]+" "+v)
			}
			put(y+4+i, 101, strings.Join(parts, "  "), dim)
		}
	}
	u.scr.Show()
}

func lastLink(hist []Sample) *Link {
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].Link != nil {
			return hist[i].Link
		}
	}
	return nil
}

func mainLoop(scr tcell.Screen) {
	u := &UI{scr: scr}
	sm := &Sampler{}
	go sm.Run()

	events := make(chan tcell.Event, 64)
	go func() {
		for {
			ev := scr.PollEvent()
			if ev == nil {
				return
			}
			events <- ev
		}
	}()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	typed := ""
	var saved []Spot
	var pending *Pending // waiting for a fresh scan
	for {
		var key *tcell.EventKey
		select {
		case ev := <-events:
			switch ev := ev.(type) {
			case *tcell.EventKey:
				key = ev
			case *tcell.EventResize:
				scr.Sync()
			}
		case <-tick.C:
		}
		if key != nil && (key.Key() == tcell.KeyEscape || key.Key() == tcell.KeyCtrlC) {
			return
		}
		if key != nil && key.Key() == tcell.KeyEnter && strings.TrimSpace(typed) != "" && pending == nil {
			hist := sm.Window(10)
			sp := Spot{Name: strings.TrimSpace(typed), St: stats(hist)}
			if l := lastLink(hist); l != nil {
				sp.Ch, sp.Unit = l.Ch, l.Unit
			}
			pending = &Pending{Spot: sp, RequestedAt: requestScan()}
			typed = ""
		}
		if pending != nil {
			// save once a scan newer than the request lands (or after 10 s, with whatever is there)
			ssid := ""
			if l := lastLink(sm.Window(10)); l != nil {
				ssid = l.SSID
			}
			sc := unitScan(ssid)
			fresh := sc != nil && now()-sc.Age > pending.RequestedAt
			if fresh || now()-pending.RequestedAt > 10 {
				pending.Scan = sc
				save(pending.Spot)
				saved = append(saved, pending.Spot)
				pending = nil
			}
		} else if key != nil {
			switch key.Key() {
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if r := []rune(typed); len(r) > 0 {
					typed = string(r[:len(r)-1])
				}
			case tcell.KeyRune:
				typed += string(key.Rune())
			}
		}
		draw(u, sm, typed, saved, pending)
	}
}
