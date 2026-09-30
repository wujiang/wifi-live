package main

import (
	"encoding/csv"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// pyFloat formats like Python's str(float): 12.0 -> "12.0", 4.3 -> "4.3".
func pyFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

func optInt(p *int, none string) string {
	if p == nil {
		return none
	}
	return strconv.Itoa(*p)
}

func optFloat(p *float64, none string) string {
	if p == nil {
		return none
	}
	return pyFloat(*p)
}

func surveyHeader() []string {
	h := []string{"time", "room", "connected_to", "signal_dBm", "rate_Mbps", "band", "channel",
		"loss_pct", "router_ping_ms", "worst_ping_ms"}
	for _, u := range UnitNames {
		h = append(h, u+" 5GHz", u+" 2.4GHz")
	}
	return append(h, "scan_age_s")
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newCSV(f *os.File) *csv.Writer {
	w := csv.NewWriter(f)
	w.UseCRLF = true // match Python's csv module
	return w
}

type Spot struct {
	Name string
	St   Stats
	Ch   int
	Unit string
	Scan *Scan
}

// save appends one spot. Older files with fewer columns are rewritten with the current header first.
func save(sp Spot) error {
	header := surveyHeader()
	if f, err := os.Open(Survey); err == nil {
		r := csv.NewReader(f)
		r.FieldsPerRecord = -1
		rows, err := r.ReadAll()
		f.Close()
		if err == nil && len(rows) > 0 && !equal(rows[0], header) {
			out, err := os.Create(Survey)
			if err != nil {
				return err
			}
			w := newCSV(out)
			w.Write(header)
			for _, row := range rows[1:] {
				for len(row) < len(header) {
					row = append(row, "")
				}
				w.Write(row)
			}
			w.Flush()
			out.Close()
		}
	}
	_, err := os.Stat(Survey)
	isNew := os.IsNotExist(err)
	f, err := os.OpenFile(Survey, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := newCSV(f)
	if isNew {
		w.Write(header)
	}
	ch := ""
	if sp.Ch != 0 {
		ch = strconv.Itoa(sp.Ch)
	}
	row := []string{time.Now().Format("2006-01-02 15:04:05"), sp.Name, sp.Unit,
		optInt(sp.St.RSSI, ""), optInt(sp.St.Rate, ""), band(sp.Ch), ch,
		optInt(sp.St.Loss, ""), optFloat(sp.St.Ping, ""), optInt(sp.St.Worst, "")}
	for _, u := range UnitNames {
		for _, b := range []string{"5", "2.4"} {
			v := ""
			if sp.Scan != nil {
				if x, ok := sp.Scan.Units[u][b]; ok {
					v = strconv.Itoa(x)
				}
			}
			row = append(row, v)
		}
	}
	age := ""
	if sp.Scan != nil {
		age = strconv.Itoa(int(math.RoundToEven(sp.Scan.Age)))
	}
	w.Write(append(row, age))
	w.Flush()
	return w.Error()
}
