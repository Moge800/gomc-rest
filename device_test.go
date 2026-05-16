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
		{"TC10", mc.DeviceAddr{Device: "TC", Addr: 10}},
		{"TS10", mc.DeviceAddr{Device: "TS", Addr: 10}},
		{"CC5", mc.DeviceAddr{Device: "CC", Addr: 5}},
		{"CS5", mc.DeviceAddr{Device: "CS", Addr: 5}},
		{"STN3", mc.DeviceAddr{Device: "STN", Addr: 3}},
		{"STC3", mc.DeviceAddr{Device: "STC", Addr: 3}},
		{"STS3", mc.DeviceAddr{Device: "STS", Addr: 3}},
		{"DX0", mc.DeviceAddr{Device: "DX", Addr: 0}},
		{"DY0", mc.DeviceAddr{Device: "DY", Addr: 0}},
		{"S10", mc.DeviceAddr{Device: "S", Addr: 10}},
		{"V5", mc.DeviceAddr{Device: "V", Addr: 5}},
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
		"TN",     // 番号なし（2文字プレフィックス）
		"STC",    // 番号なし（3文字プレフィックス）
		"Q100",   // 不明デバイス
		"Dabc",   // 番号が非数値
		"D-1",    // 負数
	}

	wordDevCases := []struct {
		dev  string
		want bool
	}{
		{"D", true}, {"TN", true}, {"STN", true}, {"CN", true},
		{"TC", false}, {"CC", false}, {"M", false}, {"X", false},
	}
	for _, tc := range wordDevCases {
		if got := isWordDevice(tc.dev); got != tc.want {
			t.Errorf("isWordDevice(%q) = %v, want %v", tc.dev, got, tc.want)
		}
	}

	for _, in := range ng {
		if _, err := parseAddr(in); err == nil {
			t.Errorf("parseAddr(%q) expected error, got nil", in)
		}
	}
}
