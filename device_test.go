package main

import (
	"testing"

	mc "github.com/moge800/gomcprotocol"
)

func TestParseAddr(t *testing.T) {
	ok := []struct {
		in   string
		want ParsedAddr
	}{
		// 10進数アドレスデバイス（Bit=-1）
		{"D100", ParsedAddr{mc.DeviceAddr{Device: "D", Addr: 100}, -1, 0}},
		{"M0", ParsedAddr{mc.DeviceAddr{Device: "M", Addr: 0}, -1, 0}},
		{"SM100", ParsedAddr{mc.DeviceAddr{Device: "SM", Addr: 100}, -1, 0}},
		{"SD200", ParsedAddr{mc.DeviceAddr{Device: "SD", Addr: 200}, -1, 0}},
		{"TN10", ParsedAddr{mc.DeviceAddr{Device: "TN", Addr: 10}, -1, 0}},
		{"CN5", ParsedAddr{mc.DeviceAddr{Device: "CN", Addr: 5}, -1, 0}},
		{"TC10", ParsedAddr{mc.DeviceAddr{Device: "TC", Addr: 10}, -1, 0}},
		{"TS10", ParsedAddr{mc.DeviceAddr{Device: "TS", Addr: 10}, -1, 0}},
		{"CC5", ParsedAddr{mc.DeviceAddr{Device: "CC", Addr: 5}, -1, 0}},
		{"CS5", ParsedAddr{mc.DeviceAddr{Device: "CS", Addr: 5}, -1, 0}},
		{"STN3", ParsedAddr{mc.DeviceAddr{Device: "STN", Addr: 3}, -1, 0}},
		{"STC3", ParsedAddr{mc.DeviceAddr{Device: "STC", Addr: 3}, -1, 0}},
		{"STS3", ParsedAddr{mc.DeviceAddr{Device: "STS", Addr: 3}, -1, 0}},
		{"S10", ParsedAddr{mc.DeviceAddr{Device: "S", Addr: 10}, -1, 0}},
		{"V5", ParsedAddr{mc.DeviceAddr{Device: "V", Addr: 5}, -1, 0}},
		// 16進数アドレスデバイス（Bit=-1）
		{"X10", ParsedAddr{mc.DeviceAddr{Device: "X", Addr: 0x10}, -1, 0}},
		{"X4F", ParsedAddr{mc.DeviceAddr{Device: "X", Addr: 0x4F}, -1, 0}},
		{"Y1", ParsedAddr{mc.DeviceAddr{Device: "Y", Addr: 0x1}, -1, 0}},
		{"Y12D2", ParsedAddr{mc.DeviceAddr{Device: "Y", Addr: 0x12D2}, -1, 0}},
		{"B18FD", ParsedAddr{mc.DeviceAddr{Device: "B", Addr: 0x18FD}, -1, 0}},
		{"W0", ParsedAddr{mc.DeviceAddr{Device: "W", Addr: 0}, -1, 0}},
		{"W1D", ParsedAddr{mc.DeviceAddr{Device: "W", Addr: 0x1D}, -1, 0}},
		{"ZR512", ParsedAddr{mc.DeviceAddr{Device: "ZR", Addr: 0x512}, -1, 0}},
		{"SB10", ParsedAddr{mc.DeviceAddr{Device: "SB", Addr: 0x10}, -1, 0}},
		{"SW5", ParsedAddr{mc.DeviceAddr{Device: "SW", Addr: 0x5}, -1, 0}},
		{"DX0", ParsedAddr{mc.DeviceAddr{Device: "DX", Addr: 0}, -1, 0}},
		{"DY0", ParsedAddr{mc.DeviceAddr{Device: "DY", Addr: 0}, -1, 0}},
		// 小文字・空白
		{"d100", ParsedAddr{mc.DeviceAddr{Device: "D", Addr: 100}, -1, 0}},
		{" M10 ", ParsedAddr{mc.DeviceAddr{Device: "M", Addr: 10}, -1, 0}},
		// ビットアクセス（16進1桁 0–F）
		{"D100.0", ParsedAddr{mc.DeviceAddr{Device: "D", Addr: 100}, 0, 0}},
		{"D100.9", ParsedAddr{mc.DeviceAddr{Device: "D", Addr: 100}, 9, 0}},
		{"D100.A", ParsedAddr{mc.DeviceAddr{Device: "D", Addr: 100}, 10, 0}},
		{"D100.F", ParsedAddr{mc.DeviceAddr{Device: "D", Addr: 100}, 15, 0}},
		{"W10.7", ParsedAddr{mc.DeviceAddr{Device: "W", Addr: 0x10}, 7, 0}},
		{"d100.3", ParsedAddr{mc.DeviceAddr{Device: "D", Addr: 100}, 3, 0}},
		{"d100.a", ParsedAddr{mc.DeviceAddr{Device: "D", Addr: 100}, 10, 0}},
		// K記法（ビットデバイスのワード化、K1–K8）
		{"K1M100", ParsedAddr{mc.DeviceAddr{Device: "M", Addr: 100}, -1, 1}},
		{"K4M100", ParsedAddr{mc.DeviceAddr{Device: "M", Addr: 100}, -1, 4}},
		{"K8M100", ParsedAddr{mc.DeviceAddr{Device: "M", Addr: 100}, -1, 8}},
		{"K3M0", ParsedAddr{mc.DeviceAddr{Device: "M", Addr: 0}, -1, 3}},
		{"K4X1F", ParsedAddr{mc.DeviceAddr{Device: "X", Addr: 0x1F}, -1, 4}},
		{"K4SM50", ParsedAddr{mc.DeviceAddr{Device: "SM", Addr: 50}, -1, 4}},
		{"k4m100", ParsedAddr{mc.DeviceAddr{Device: "M", Addr: 100}, -1, 4}},
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
		"",         // 短すぎ
		"D",        // 番号なし
		"TN",       // 番号なし（2文字プレフィックス）
		"STC",      // 番号なし（3文字プレフィックス）
		"Q100",     // 不明デバイス
		"T10",      // 単一文字 T は無効（TC を使う）
		"C5",       // 単一文字 C は無効（CC を使う）
		"Dabc",     // D は10進数、abc は無効
		"D-1",      // 負数
		"X-1",      // X は16進数、負数は無効
		"XGGGG",    // 16進数として無効
		"M0.0",     // ビットデバイスにビット指定不可
		"D100.10",  // 2文字（10進表記）は不可
		"D100.15",  // 2文字（10進表記）は不可
		"D100.G",   // 16進として無効
		"D100.-1",  // 2文字かつ無効
		"D.0",      // アドレス番号なし
		"K0M100",   // ニブル数 0 は無効
		"K9M100",   // ニブル数 9 は無効
		"K4D100",   // K記法はワードデバイス不可
		"K4W10",    // 同上（16進ワードデバイス）
		"K4M100.1", // K記法とビット指定(.N)の併用不可
		"K4",       // デバイス指定なし
		"K4Q100",   // 不明デバイス
	}

	for _, in := range ng {
		if _, err := parseAddr(in); err == nil {
			t.Errorf("parseAddr(%q) expected error, got nil", in)
		}
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
}
