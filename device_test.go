package main

import (
	"testing"

	mc "github.com/moge800/gomcprotocol"
)

func TestParseAddr(t *testing.T) {
	ok := []struct {
		in   string
		want mc.DeviceAddr
	}{
		// 1文字プレフィックス
		{"D100", mc.DeviceAddr{Device: "D", Addr: 100}},
		{"W0", mc.DeviceAddr{Device: "W", Addr: 0}},
		{"X10", mc.DeviceAddr{Device: "X", Addr: 10}},
		{"Y1", mc.DeviceAddr{Device: "Y", Addr: 1}},
		{"M0", mc.DeviceAddr{Device: "M", Addr: 0}},
		// 2文字プレフィックス
		{"ZR512", mc.DeviceAddr{Device: "ZR", Addr: 512}},
		{"SB10", mc.DeviceAddr{Device: "SB", Addr: 10}},
		{"SW5", mc.DeviceAddr{Device: "SW", Addr: 5}},
		{"SM100", mc.DeviceAddr{Device: "SM", Addr: 100}},
		{"SD200", mc.DeviceAddr{Device: "SD", Addr: 200}},
		{"TN10", mc.DeviceAddr{Device: "TN", Addr: 10}},
		{"CN5", mc.DeviceAddr{Device: "CN", Addr: 5}},
		{"T10", mc.DeviceAddr{Device: "T", Addr: 10}},
		{"C5", mc.DeviceAddr{Device: "C", Addr: 5}},
		// 小文字・空白
		{"d100", mc.DeviceAddr{Device: "D", Addr: 100}},
		{" M10 ", mc.DeviceAddr{Device: "M", Addr: 10}},
	}

	for _, tc := range ok {
		got, err := parseAddr(tc.in)
		if err != nil {
			t.Errorf("parseAddr(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseAddr(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}

	ng := []string{
		"",       // 短すぎ
		"D",      // 番号なし
		"Q100",   // 不明デバイス
		"Dabc",   // 番号が非数値
		"D-1",    // 負数
	}

	for _, in := range ng {
		if _, err := parseAddr(in); err == nil {
			t.Errorf("parseAddr(%q) expected error, got nil", in)
		}
	}
}
