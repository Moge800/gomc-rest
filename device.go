package main

import (
	"fmt"
	"math"
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

var hexAddrDevs = map[string]bool{
	"X": true, "Y": true, "B": true, "SB": true,
	"W": true, "SW": true, "ZR": true, "DX": true, "DY": true,
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

// ParsedAddr extends mc.DeviceAddr with an optional bit index for word devices
// and an optional nibble count for K notation.
// Bit == -1 means no bit access; 0–15 means access that bit of the word register.
// Nibbles == 0 means no K notation; 1–8 means pack that many nibbles
// (Nibbles*4 consecutive bit devices) into a single integer.
type ParsedAddr struct {
	mc.DeviceAddr
	Bit     int
	Nibbles int
}

func parseAddr(s string) (ParsedAddr, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	full := s // whole normalized address, so errors still quote it after the K prefix is stripped

	// K notation packs consecutive bit devices into one integer, e.g. K4M100
	// covers M100–M115 as a single 16-bit value. Strip the "K<n>" prefix and
	// parse the remainder as an ordinary device address.
	nibbles := 0
	if len(s) > 2 && s[0] == 'K' && s[1] >= '0' && s[1] <= '9' {
		if s[1] < '1' || s[1] > '8' {
			return ParsedAddr{}, fmt.Errorf("invalid nibble count in %q: K notation supports K1 through K8", s)
		}
		nibbles = int(s[1] - '0')
		s = s[2:]
	}

	if len(s) < 2 {
		return ParsedAddr{}, fmt.Errorf("invalid device address: %q", full)
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
		return ParsedAddr{}, fmt.Errorf("unknown device: %q", dev)
	}
	if rest == "" {
		return ParsedAddr{}, fmt.Errorf("missing address number in %q", s)
	}

	// Split optional bit suffix, e.g. "3500.A" → addrPart="3500", bitPart="A"
	bit := -1
	addrPart := rest
	if i := strings.IndexByte(rest, '.'); i >= 0 {
		if !wordDevs[dev] {
			return ParsedAddr{}, fmt.Errorf("bit access (.N) is only supported for word devices, not %q", dev)
		}
		bitStr := rest[i+1:]
		if len(bitStr) != 1 {
			return ParsedAddr{}, fmt.Errorf("invalid bit index in %q: must be a single hex digit 0–F", s)
		}
		bitN, err := strconv.ParseInt(bitStr, 16, 64)
		if err != nil {
			return ParsedAddr{}, fmt.Errorf("invalid bit index in %q: must be a single hex digit 0–F", s)
		}
		bit = int(bitN)
		addrPart = rest[:i]
		if addrPart == "" {
			return ParsedAddr{}, fmt.Errorf("missing address number in %q", s)
		}
	}

	// K notation is a bit-device packing, so a word device is a caller error.
	// A bit suffix cannot reach here alongside it: the .N block above already
	// rejects .N on bit devices, and word devices are rejected right here.
	if nibbles > 0 && wordDevs[dev] {
		return ParsedAddr{}, fmt.Errorf("K notation is only supported for bit devices, not %q", dev)
	}

	var addr int
	if hexAddrDevs[dev] {
		n, err := strconv.ParseInt(addrPart, 16, 64)
		if err != nil || n < 0 || n > math.MaxInt {
			return ParsedAddr{}, fmt.Errorf("invalid address number in %q (hexadecimal expected)", s)
		}
		addr = int(n)
	} else {
		n, err := strconv.Atoi(addrPart)
		if err != nil || n < 0 {
			return ParsedAddr{}, fmt.Errorf("invalid address number in %q", s)
		}
		addr = n
	}

	return ParsedAddr{DeviceAddr: mc.DeviceAddr{Device: dev, Addr: addr}, Bit: bit, Nibbles: nibbles}, nil
}

func isWordDevice(dev string) bool {
	return wordDevs[dev]
}
