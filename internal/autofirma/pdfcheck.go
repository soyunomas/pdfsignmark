package autofirma

import (
	"fmt"
	"os"
	"strings"
)

type VisibleCheck struct {
	LikelyVisible bool
	HasSignature  bool
	HasWidget     bool
	HasAppearance bool
	HasRect       bool
	Note          string
}

func (v VisibleCheck) String() string {
	parts := []string{
		fmt.Sprintf("firma=%t", v.HasSignature),
		fmt.Sprintf("widget=%t", v.HasWidget),
		fmt.Sprintf("apariencia=/AP=%t", v.HasAppearance),
		fmt.Sprintf("rect=/Rect=%t", v.HasRect),
		fmt.Sprintf("probableVisible=%t", v.LikelyVisible),
	}
	if strings.TrimSpace(v.Note) != "" {
		parts = append(parts, "nota="+v.Note)
	}
	return strings.Join(parts, "; ")
}

// CheckVisibleSignature performs a conservative byte-level inspection of the
// signed PDF. It does not validate cryptography. It only tries to detect whether
// the PDF contains a visible signature widget with an appearance stream. Some
// PDFs can compress these dictionaries, so this is a diagnostic heuristic.
func CheckVisibleSignature(pdfPath string) VisibleCheck {
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return VisibleCheck{Note: "no se pudo leer el PDF firmado: " + err.Error()}
	}
	// Limit memory already read; AutoFirma/iText usually appends signature objects
	// uncompressed near the end of the file. Keep full scan for robustness.
	s := string(data)
	check := VisibleCheck{
		HasSignature: strings.Contains(s, "/FT/Sig") || strings.Contains(s, "/FT /Sig") || strings.Contains(s, "/Type/Sig") || strings.Contains(s, "/Type /Sig"),
		HasWidget:    strings.Contains(s, "/Subtype/Widget") || strings.Contains(s, "/Subtype /Widget"),
		HasAppearance: strings.Contains(s, "/AP<<") || strings.Contains(s, "/AP <<") || strings.Contains(s, "/AP ") ||
			strings.Contains(s, "/N "),
		HasRect: strings.Contains(s, "/Rect[") || strings.Contains(s, "/Rect ["),
	}
	check.LikelyVisible = check.HasSignature && check.HasWidget && check.HasAppearance && check.HasRect
	if !check.LikelyVisible {
		check.Note = "heurística; si el PDF usa objetos comprimidos puede dar falso negativo"
	}
	return check
}
