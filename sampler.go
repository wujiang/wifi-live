package main

import (
	"math"
	"os"
	"sort"
	"sync"
	"time"
)

type Sample struct {
	T      float64
	Link   *Link
	Router *float64
	Net    *float64
}

type Roam struct {
	T        float64
	From, To string
	RSSI     int
}

// Sampler collects one sample per second in the background.
type Sampler struct {
	mu    sync.Mutex
	hist  []Sample // last 120
	roams []Roam   // last 5
}

func (s *Sampler) Run() {
	for {
		start := time.Now()
		t := now()
		var link *Link
		var r, n *float64
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); link = wifiLink() }()
		go func() { defer wg.Done(); r = ping(Router) }()
		go func() { defer wg.Done(); n = ping(Internet) }()
		wg.Wait()
		tn := time.Now()
		os.Chtimes(WantFile, tn, tn)
		if link != nil && link.BSSID != "" {
			if link.Unit = unitFor(link.BSSID); link.Unit == "" {
				link.Unit = "unknown unit"
			}
		}
		s.mu.Lock()
		var prev *Link
		for i := len(s.hist) - 1; i >= 0; i-- {
			if s.hist[i].Link != nil {
				prev = s.hist[i].Link
				break
			}
		}
		if link != nil && link.Unit != "" && prev != nil && prev.Unit != "" && prev.Unit != link.Unit {
			s.roams = append(s.roams, Roam{t, prev.Unit, link.Unit, link.RSSI})
			if len(s.roams) > 5 {
				s.roams = s.roams[1:]
			}
		}
		s.hist = append(s.hist, Sample{t, link, r, n})
		if len(s.hist) > 120 {
			s.hist = s.hist[1:]
		}
		s.mu.Unlock()
		time.Sleep(time.Second - time.Since(start))
	}
}

func (s *Sampler) Window(secs float64) []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := now()
	var out []Sample
	for _, h := range s.hist {
		if t-h.T <= secs {
			out = append(out, h)
		}
	}
	return out
}

func (s *Sampler) LastRoam() *Roam {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.roams) == 0 {
		return nil
	}
	r := s.roams[len(s.roams)-1]
	return &r
}

type Stats struct {
	RSSI, Rate, Loss, Worst *int
	Ping                    *float64
}

func iptr(v int) *int { return &v }

func stats(samples []Sample) Stats {
	var st Stats
	var rssi, rate, ok []float64
	for _, s := range samples {
		if s.Link != nil {
			rssi = append(rssi, float64(s.Link.RSSI))
			rate = append(rate, float64(s.Link.Rate))
		}
		if s.Router != nil {
			ok = append(ok, *s.Router)
		}
	}
	avg := func(xs []float64) *int {
		if len(xs) == 0 {
			return nil
		}
		sum := 0.0
		for _, x := range xs {
			sum += x
		}
		return iptr(int(math.RoundToEven(sum / float64(len(xs)))))
	}
	st.RSSI, st.Rate = avg(rssi), avg(rate)
	if len(samples) > 0 {
		st.Loss = iptr(int(math.RoundToEven(100 * float64(len(samples)-len(ok)) / float64(len(samples)))))
	}
	if len(ok) > 0 {
		sorted := append([]float64(nil), ok...)
		sort.Float64s(sorted)
		p := math.RoundToEven(sorted[len(sorted)/2]*10) / 10
		st.Ping = &p
		st.Worst = iptr(int(math.RoundToEven(sorted[len(sorted)-1])))
	}
	return st
}
