package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// Config is read from wifi_live.json next to the binary (see wifi_live.example.json).
// The first unit is the router, which gets pinged. Without a config file the default
// gateway is used as the only unit.
type Config struct {
	Internet string       `json:"internet"`
	Units    []UnitConfig `json:"units"`
}

type UnitConfig struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	MAC  string `json:"mac"` // optional; used when a unit is offline and not in the ARP table
}

var (
	Router, RouterName string
	Internet           = "1.1.1.1"
	UnitNames          []string              // mesh units in display order; their MACs are looked up to recognize BSSIDs
	Units              = map[string]string{} // name -> IP
	KnownMACs          = map[string]string{} // name -> MAC
)

func loadConfig() error {
	var c Config
	data, err := os.ReadFile(ConfigFile)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &c); err != nil {
			return fmt.Errorf("%s: %w", ConfigFile, err)
		}
	case os.IsNotExist(err):
		out, _ := run("route", "-n", "get", "default")
		m := regexp.MustCompile(`gateway: (\S+)`).FindStringSubmatch(out)
		if m == nil {
			return fmt.Errorf("no %s and no default gateway found", ConfigFile)
		}
		c.Units = []UnitConfig{{Name: "Router", IP: m[1]}}
	default:
		return err
	}
	if len(c.Units) == 0 {
		return fmt.Errorf("%s: no units", ConfigFile)
	}
	if c.Internet != "" {
		Internet = c.Internet
	}
	for _, u := range c.Units {
		UnitNames = append(UnitNames, u.Name)
		Units[u.Name] = u.IP
		if u.MAC != "" {
			KnownMACs[u.Name] = u.MAC
		}
	}
	RouterName, Router = c.Units[0].Name, c.Units[0].IP
	return nil
}
