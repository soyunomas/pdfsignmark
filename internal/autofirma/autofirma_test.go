package autofirma

import (
	"strings"
	"testing"
)

func TestBuildConfigDefaultsToMinimalVisibleLiteralBackslashN(t *testing.T) {
	cfg := BuildConfig(SignOptions{
		Rect:    Rect{Page: 2, LLX: 10, LLY: 20, URX: 200, URY: 100},
		Visible: true,
	})
	if strings.Contains(cfg, "\n") {
		t.Fatalf("config should use literal backslash-n by default, not real newlines: %q", cfg)
	}
	for _, want := range []string{
		"signaturePositionOnPageUpperRightY=100",
		"signaturePositionOnPageUpperRightX=200",
		"signaturePositionOnPageLowerLeftY=20",
		"signaturePositionOnPageLowerLeftX=10",
		"signaturePage=2",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("missing %q in config: %q", want, cfg)
		}
	}
	for _, notWant := range []string{"layer2Text", "layer2Font", "headLess", "visibleSignature"} {
		if strings.Contains(cfg, notWant) {
			t.Fatalf("minimal visible config should not contain %q: %q", notWant, cfg)
		}
	}
}

func TestBuildConfigCanUseRealNewlines(t *testing.T) {
	cfg := BuildConfig(SignOptions{
		Rect:            Rect{Page: 2, LLX: 10, LLY: 20, URX: 200, URY: 100},
		ConfigSeparator: "newline",
		Visible:         true,
	})
	if !strings.Contains(cfg, "\nsignaturePositionOnPageUpperRightX=200\n") {
		t.Fatalf("config is not separated with real newlines: %q", cfg)
	}
}

func TestBuildConfigRoundsVisibleCoordinatesToIntegers(t *testing.T) {
	cfg := BuildConfig(SignOptions{
		Rect:    Rect{Page: 1, LLX: 56.8, LLY: 424.79, URX: 276.8, URY: 504.79},
		Visible: true,
	})
	for _, want := range []string{
		"signaturePositionOnPageUpperRightY=505",
		"signaturePositionOnPageUpperRightX=277",
		"signaturePositionOnPageLowerLeftY=425",
		"signaturePositionOnPageLowerLeftX=57",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("missing rounded integer coordinate %q in config: %q", want, cfg)
		}
	}
	if strings.Contains(cfg, ".") {
		t.Fatalf("AutoFirma visible coordinates must be integer-only, got: %q", cfg)
	}
}

func TestBuildConfigAddsLayer2TextOnlyWhenRequested(t *testing.T) {
	cfg := BuildConfig(SignOptions{
		Rect:       Rect{Page: 2, LLX: 10, LLY: 20, URX: 200, URY: 100},
		Layer2Text: "Firmado por Test",
		FontSize:   9,
		Visible:    true,
	})
	if strings.Contains(cfg, "Firmado por Test") {
		t.Fatalf("spaces should be normalized for Linux AutoFirma wrapper compatibility: %q", cfg)
	}
	if !strings.Contains(cfg, "layer2Text=Firmado_por_Test") {
		t.Fatalf("layer2Text not normalized as expected: %q", cfg)
	}
	if !strings.Contains(cfg, "layer2FontSize=9") {
		t.Fatalf("font params should be present when layer2Text is requested: %q", cfg)
	}
}

func TestBuildConfigInvisibleNoConfigUnlessHeadlessOrExtra(t *testing.T) {
	cfg := BuildConfig(SignOptions{
		Rect:       Rect{Page: 2, LLX: 10, LLY: 20, URX: 200, URY: 100},
		Layer2Text: "Firmado por Test",
		FontSize:   9,
		Visible:    false,
	})
	if cfg != "" {
		t.Fatalf("invisible signing should not send unnecessary config by default: %q", cfg)
	}

	cfg = BuildConfig(SignOptions{Headless: true, Visible: false})
	if cfg != "headLess=true" {
		t.Fatalf("headless config mismatch: %q", cfg)
	}
}

func TestShellQuoteRedacted(t *testing.T) {
	got := ShellQuoteRedacted([]string{"AutoFirma", "sign", "-password", "secret value", "-i", "a.pdf"})
	if strings.Contains(got, "secret") {
		t.Fatalf("password leaked: %s", got)
	}
	if !strings.Contains(got, "-password") || !strings.Contains(got, "***") {
		t.Fatalf("redacted command missing expected tokens: %s", got)
	}
}
