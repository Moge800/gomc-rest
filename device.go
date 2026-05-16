package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	mc "github.com/moge800/gomcprotocol"
)

var validDevs = map[string]bool{
	// word
	"D": true, "W": true, "R": true, "ZR": true,
	"TN": true, "STN": true, "CN": true, "Z": true, "SW": true, "SD": true,
	// bit
	"X": true, "Y": true, "M": true, "L": true, "B": true, "F": true, "V": true,
	"SB": true, "SM": true, "S": true, "DX": true, "DY": true,
	"TC": true, "TS": true, "STC": true, "STS": true,
	"CC": true, "CS": true,
}

var wordDevs = map[string]bool{
	"D": true, "W": true, "R": true, "ZR": true,
	"TN": true, "STN": true, "CN": true, "Z": true, "SW": true, "SD": true,
}

// multiCharPrefixes holds all device names longer than one character, sorted
// longest-first so that e.g. "STC" is matched before "S".
var multiCharPrefixes = func() []string {
	var ps []string
	for dev := range validDevs {
		if len(dev) > 1 {
			ps = append(ps, dev)
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		return len(ps[i]) > len(ps[j])
	})
	return ps
}()

func parseAddr(s string) (mc.DeviceAddr, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) < 2 {
		return mc.DeviceAddr{}, fmt.Errorf("invalid device address: %q", s)
	}

	var dev string
	var rest string

	for _, p := range multiCharPrefixes {
		if strings.HasPrefix(s, p) {
			dev = p
			rest = s[len(p):]
			break
		}
	}
	if dev == "" {
		dev = s[:1]
		rest = s[1:]
	}

	if !validDevs[dev] {
		return mc.DeviceAddr{}, fmt.Errorf("unknown device: %q", dev)
	}
	if rest == "" {
		return mc.DeviceAddr{}, fmt.Errorf("missing address number in %q", s)
	}

	addr, err := strconv.Atoi(rest)
	if err != nil || addr < 0 {
		return mc.DeviceAddr{}, fmt.Errorf("invalid address number in %q", s)
	}

	return mc.DeviceAddr{Device: dev, Addr: addr}, nil
}

func isWordDevice(dev string) bool {
	return wordDevs[dev]
}
