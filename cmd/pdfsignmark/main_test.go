package main

import "testing"

func TestParseArgsDefaultOutDirIsCurrentDirectory(t *testing.T) {
	cfg, inputs, err := parseArgs([]string{"documento.pdf"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if cfg.outDir != "." {
		t.Fatalf("default outDir = %q, want %q", cfg.outDir, ".")
	}
	if len(inputs) != 1 || inputs[0] != "documento.pdf" {
		t.Fatalf("inputs = %#v, want documento.pdf", inputs)
	}
}

func TestOutputPathDefaultDirectoryUsesCurrentWorkingDirectory(t *testing.T) {
	got := outputPath(".", "entrada/documento.pdf", "_firmado")
	if got != "documento_firmado.pdf" {
		t.Fatalf("outputPath = %q, want documento_firmado.pdf", got)
	}
}

func TestParseAliasListXML(t *testing.T) {
	got := parseAliasList(`<aliases><alias>Certificado A</alias><certificate alias="Certificado B"/></aliases>`)
	want := []string{"Certificado A", "Certificado B"}
	if len(got) != len(want) {
		t.Fatalf("parseAliasList length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseAliasList[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestParseAliasListPlainText(t *testing.T) {
	got := parseAliasList("Certificado A\n\nCertificado B\nCertificado A\n")
	want := []string{"Certificado A", "Certificado B"}
	if len(got) != len(want) {
		t.Fatalf("parseAliasList length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseAliasList[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}
