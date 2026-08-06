package device

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestTelegramDesktop(t *testing.T) {
	p := TelegramDesktop()
	if p.LangPack != "tdesktop" {
		t.Errorf("expected LangPack 'tdesktop', got %q", p.LangPack)
	}
}

func TestTelegramAndroid(t *testing.T) {
	p := TelegramAndroid()
	if p.LangPack != "android" {
		t.Errorf("expected LangPack 'android', got %q", p.LangPack)
	}
}

func TestTelegramIOS(t *testing.T) {
	p := TelegramIOS()
	if p.LangPack != "ios" {
		t.Errorf("expected LangPack 'ios', got %q", p.LangPack)
	}
}

func TestTelegramMacOS(t *testing.T) {
	p := TelegramMacOS()
	if p.LangPack != "macos" {
		t.Errorf("expected LangPack 'macos', got %q", p.LangPack)
	}
}

func TestTelegramWebZ(t *testing.T) {
	p := TelegramWebZ()
	if p.AppVersion != "1.28.3 Z" {
		t.Errorf("expected AppVersion '1.28.3 Z', got %q", p.AppVersion)
	}
}

func TestTelegramWebK(t *testing.T) {
	p := TelegramWebK()
	if p.LangPack != "macos" {
		t.Errorf("expected LangPack 'macos', got %q", p.LangPack)
	}
}

func TestDeviceGenerateAndroid(t *testing.T) {
	p := Android.Generate("test-session")
	if p.DeviceModel == "" {
		t.Error("expected non-empty DeviceModel")
	}
	if p.SystemVersion == "" {
		t.Error("expected non-empty SystemVersion")
	}
}

func TestDeviceGenerateIOS(t *testing.T) {
	p := IOS.Generate("test-session")
	if p.DeviceModel == "" {
		t.Error("expected non-empty DeviceModel")
	}
}

func TestDeviceGenerateWindows(t *testing.T) {
	p := Windows.Generate("test-session")
	if p.DeviceModel == "" {
		t.Error("expected non-empty DeviceModel")
	}
}

func TestDeviceGenerateLinux(t *testing.T) {
	p := Linux.Generate("test-session")
	if p.DeviceModel == "" {
		t.Error("expected non-empty DeviceModel")
	}
}

func TestDeviceGenerateMacOS(t *testing.T) {
	p := MacOS.Generate("test-session")
	if p.DeviceModel == "" {
		t.Error("expected non-empty DeviceModel")
	}
}

func TestDeviceGenerateDesktop(t *testing.T) {
	p := Desktop.Generate("test-session")
	if p.DeviceModel == "" {
		t.Error("expected non-empty DeviceModel")
	}
}

func TestDeviceGenerateWebZ(t *testing.T) {
	p := WebZ.Generate("")
	if p.AppVersion != "1.28.3 Z" {
		t.Errorf("expected AppVersion '1.28.3 Z', got %q", p.AppVersion)
	}
}

func TestDeviceGenerateUnknown(t *testing.T) {
	p := Device("unknown").Generate("test-session")
	if p.DeviceModel == "" {
		t.Error("expected fallback to generate non-empty DeviceModel")
	}
}

func TestDeviceGenerateDeterministic(t *testing.T) {
	p1 := Windows.Generate("same-id")
	p2 := Windows.Generate("same-id")
	if p1.DeviceModel != p2.DeviceModel {
		t.Errorf("expected same DeviceModel, got %q and %q", p1.DeviceModel, p2.DeviceModel)
	}
	if p1.SystemVersion != p2.SystemVersion {
		t.Errorf("expected same SystemVersion, got %q and %q", p1.SystemVersion, p2.SystemVersion)
	}
}

func TestProfileCopy(t *testing.T) {
	p := TelegramDesktop()
	cp := p.Copy()
	if cp.DeviceModel != p.DeviceModel {
		t.Error("copy should have same DeviceModel")
	}
	cp.DeviceModel = "modified"
	if p.DeviceModel == "modified" {
		t.Error("original should not be affected by copy modification")
	}
}

func TestProfileWithDevice(t *testing.T) {
	p := TelegramDesktop()
	modified := p.WithDevice("TestModel", "TestOS")
	if modified.DeviceModel != "TestModel" {
		t.Errorf("expected DeviceModel 'TestModel', got %q", modified.DeviceModel)
	}
	if modified.SystemVersion != "TestOS" {
		t.Errorf("expected SystemVersion 'TestOS', got %q", modified.SystemVersion)
	}
	if p.DeviceModel == "TestModel" {
		t.Error("original should not be modified")
	}
}

func TestProfileString(t *testing.T) {
	p := TelegramAndroid()
	s := p.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestProfileApply(t *testing.T) {
	p := TelegramAndroid()
	var cfg InitConnectionConfig
	p.Apply(&cfg)

	if cfg.DeviceModel != p.DeviceModel {
		t.Errorf("expected DeviceModel %q, got %q", p.DeviceModel, cfg.DeviceModel)
	}
	if cfg.SystemVersion != p.SystemVersion {
		t.Errorf("expected SystemVersion %q, got %q", p.SystemVersion, cfg.SystemVersion)
	}
	if cfg.LanguagePack != p.LangPack {
		t.Errorf("expected LanguagePack %q, got %q", p.LangPack, cfg.LanguagePack)
	}
}

func TestProfileApplyAllFields(t *testing.T) {
	p := TelegramIOS()
	var cfg InitConnectionConfig
	p.Apply(&cfg)

	if cfg.DeviceModel != p.DeviceModel {
		t.Error("Apply should set DeviceModel")
	}
	if cfg.LanguageCode != p.LangCode {
		t.Error("Apply should set LanguageCode")
	}
	if cfg.SystemLanguageCode != p.SystemLangCode {
		t.Error("Apply should set SystemLanguageCode")
	}
}

func TestToInitConnection(t *testing.T) {
	p := TelegramMacOS()
	ic := p.ToInitConnection()

	if ic.DeviceModel != p.DeviceModel {
		t.Errorf("expected DeviceModel %q, got %q", p.DeviceModel, ic.DeviceModel)
	}
	if ic.AppVersion != p.AppVersion {
		t.Errorf("expected AppVersion %q, got %q", p.AppVersion, ic.AppVersion)
	}
	if ic.LanguagePack != p.LangPack {
		t.Errorf("expected LanguagePack %q, got %q", p.LangPack, ic.LanguagePack)
	}
}

func TestGenerateAndroidNonEmpty(t *testing.T) {
	for i := range 100 {
		p := GenerateAndroid("session-" + string(rune('a'+i%26)))
		if p.DeviceModel == "" || p.SystemVersion == "" {
			t.Fatalf("iter %d: expected non-empty device fields", i)
		}
	}
}


func TestTelegramPlus(t *testing.T) {
	p := TelegramPlus()
	if p.LangPack != "android" {
		t.Errorf("expected LangPack 'android', got %q", p.LangPack)
	}
}

func TestDeviceGeneratePlus(t *testing.T) {
	p := Plus.Generate("test-session")
	if p.DeviceModel == "" {
		t.Error("expected non-empty DeviceModel")
	}
	if p.SystemVersion == "" {
		t.Error("expected non-empty SystemVersion")
	}
}
func TestTelegramWebogram(t *testing.T) {
	p := TelegramWebogram()
	if p.AppVersion != "0.7.0" {
		t.Errorf("expected AppVersion '0.7.0', got %q", p.AppVersion)
	}
}

func TestDeviceGenerateWebogram(t *testing.T) {
	p := Webogram.Generate("test")
	if p.AppVersion != "0.7.0" {
		t.Errorf("expected AppVersion '0.7.0', got %q", p.AppVersion)
	}
}

func TestConcurrentGenerate(t *testing.T) {
	// All device types — exercises every lazy-init path concurrently.
	devices := []Device{Android, AndroidX, Plus, IOS, MacOS, Windows, Linux, Desktop}

	var wg sync.WaitGroup
	for range 200 {
		for _, d := range devices {
			wg.Add(1)
			go func(d Device) {
				defer wg.Done()
				p := d.Generate("concurrent-test")
				if p.DeviceModel == "" {
					t.Error("expected non-empty DeviceModel")
				}
			}(d)
		}
	}
	wg.Wait()
}

func TestMacOSFromIdentifier(t *testing.T) {
	tests := []struct {
		identifier string
		want       string
	}{
		{"MacBookPro16,4", "MacBook Pro"},
		{"MacBookAir10,1", "MacBook Air"},
		{"MacBook10,1", "MacBook"},
		{"iMac20,2", "iMac"},
		{"iMacPro1,1", "iMac Pro"},
		{"Macmini9,1", "Mac mini"},
		{"MacPro7,1", "Mac Pro"},
	}

	for _, tt := range tests {
		t.Run(tt.identifier, func(t *testing.T) {
			if got := macOSFromIdentifier(tt.identifier); got != tt.want {
				t.Errorf("macOSFromIdentifier(%q) = %q, want %q", tt.identifier, got, tt.want)
			}
		})
	}
}

func TestMacOSDeviceModelsCanonical(t *testing.T) {
	canonical := map[string]bool{
		"MacBook Pro": true,
		"MacBook Air": true,
		"MacBook":     true,
		"iMac":        true,
		"iMac Pro":    true,
		"Mac mini":    true,
		"Mac Pro":     true,
	}

	for _, model := range macOSDeviceModels {
		if model == "" {
			t.Error("macOSDeviceModels contains an empty model")
		}
		if strings.Contains(model, ",") {
			t.Errorf("macOSDeviceModels contains identifier punctuation: %q", model)
		}
		if !canonical[model] {
			t.Errorf("macOSDeviceModels contains non-canonical model %q", model)
		}
	}
}

func TestIOSDeviceSelectionStable(t *testing.T) {
	tests := []struct {
		id   string
		want deviceInfo
	}{
		{"stable-id", deviceInfo{model: "iPhone 6 Plus", version: "12.1.3"}},
		{"session-1", deviceInfo{model: "iPhone XR", version: "14.4.1"}},
		{"session-2", deviceInfo{model: "iPhone XR", version: "15.2"}},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := randomIOSDevice(tt.id); got != tt.want {
				t.Errorf("randomIOSDevice(%q) = %+v, want %+v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIOSDeviceListOrder(t *testing.T) {
	versionsByMajor := map[int][]string{
		12: {
			"12.0.1",
			"12.1.4", "12.1.3", "12.1.2", "12.1.1",
			"12.2",
			"12.3.2", "12.3.1",
			"12.4.9", "12.4.8", "12.4.7", "12.4.6", "12.4.5", "12.4.4", "12.4.3", "12.4.2", "12.4.1",
			"12.5.5", "12.5.4", "12.5.3", "12.5.2", "12.5.1",
			"12.11.0",
		},
		13: {
			"13.1.3", "13.1.2", "13.1.1",
			"13.2.3", "13.2.2",
			"13.3.1",
			"13.4.1",
			"13.5.1",
			"13.6.1",
			"13.7",
		},
		14: {
			"14.0.1",
			"14.1",
			"14.2.1",
			"14.3",
			"14.4.2", "14.4.1",
			"14.5.1",
			"14.6",
			"14.7.1",
			"14.8.1",
		},
		15: {
			"15.0.2", "15.0.1",
			"15.1.1",
			"15.2",
		},
	}

	list := initIOSDeviceList()
	index := 0
	for _, entry := range iOSDeviceModels {
		for _, suffix := range entry.Suffixes {
			model := fmt.Sprintf("iPhone %d%s", entry.ID, suffix)
			if entry.ID == 10 {
				model = "iPhone X" + suffix
			}
			for _, major := range iosAvailableVersions(entry.ID) {
				for _, version := range versionsByMajor[major] {
					if index >= len(list) {
						t.Fatalf("iOS device list ended at index %d", index)
					}
					want := deviceInfo{model: model, version: version}
					if list[index] != want {
						t.Fatalf("iOS device list entry %d = %+v, want %+v", index, list[index], want)
					}
					index++
				}
			}
		}
	}
	if index != len(list) {
		t.Fatalf("iOS device list has %d unexpected trailing entries", len(list)-index)
	}
}
