package main

import (
	"fmt"
	"strconv"
	"strings"

	mc "github.com/moge800/gomcprotocol"
)

var validDevs = map[string]bool{
	"D": true, "W": true, "R": true, "ZR": true,
	"X": true, "Y": true, "M": true, "L": true,
	"B": true, "F": true, "SB": true, "SW": true,
	"SM": true, "SD": true, "TN": true, "CN": true, "Z": true,
}

var wordDevs = map[string]bool{
	"D": true, "W": true, "R": true, "ZR": true,
	"TN": true, "CN": true, "Z": true, "SW": true, "SD": true,
}

// two-char prefixes tried before single-char
var twoCharPrefixes = []string{"ZR", "SB", "SW", "SM", "SD", "TN", "CN"}

func parseAddr(s string) (mc.DeviceAddr, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) < 2 {
		return mc.DeviceAddr{}, fmt.Errorf("invalid device address: %q", s)
	}

	var dev string
	var rest string

	for _, p := range twoCharPrefixes {
		if strings.HasPrefix(s, p) && len(s) > len(p) {
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
