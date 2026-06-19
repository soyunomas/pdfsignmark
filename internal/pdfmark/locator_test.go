package pdfmark

import "testing"

func TestParseAndFindMarkerSingleWord(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html><body><doc><page width="612" height="792">
<word xMin="100" yMin="200" xMax="150" yMax="214">[FIRMA]</word>
</page></doc></body></html>`)
	words, err := ParseBBoxWords(data)
	if err != nil {
		t.Fatal(err)
	}
	matches := FindMarker(words, "[FIRMA]", false)
	if len(matches) != 1 {
		t.Fatalf("matches=%d, want 1", len(matches))
	}
	r := RectForMatch(matches[0], 220, 80, 0, 0, 0, AnchorLowerLeft)
	if r.Page != 1 || r.LLX != 100 || r.LLY != 578 || r.URX != 320 || r.URY != 658 {
		t.Fatalf("rect=%+v", r)
	}
}

func TestFindMarkerSplitWords(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html><body><doc><page width="612" height="792">
<word xMin="10" yMin="20" xMax="15" yMax="30">[</word>
<word xMin="16" yMin="20" xMax="50" yMax="30">FIRMA</word>
<word xMin="51" yMin="20" xMax="56" yMax="30">]</word>
</page></doc></body></html>`)
	words, err := ParseBBoxWords(data)
	if err != nil {
		t.Fatal(err)
	}
	matches := FindMarker(words, "[FIRMA]", false)
	if len(matches) != 1 {
		t.Fatalf("matches=%d, want 1", len(matches))
	}
	if matches[0].BoxTopLeft.XMin != 10 || matches[0].BoxTopLeft.XMax != 56 {
		t.Fatalf("unexpected union: %+v", matches[0].BoxTopLeft)
	}
}

func TestFindMarkerPhraseWithSpaces(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html><body><doc><page width="612" height="792">
<word xMin="100" yMin="200" xMax="145" yMax="214">Firmado</word>
<word xMin="150" yMin="200" xMax="170" yMax="214">por</word>
<word xMin="175" yMin="200" xMax="230" yMax="214">solicitante</word>
</page></doc></body></html>`)
	words, err := ParseBBoxWords(data)
	if err != nil {
		t.Fatal(err)
	}
	matches := FindMarker(words, "Firmado por solicitante", false)
	if len(matches) != 1 {
		t.Fatalf("matches=%d, want 1", len(matches))
	}
	if matches[0].BoxTopLeft.XMin != 100 || matches[0].BoxTopLeft.XMax != 230 {
		t.Fatalf("unexpected union: %+v", matches[0].BoxTopLeft)
	}
}

func TestFindMarkerPhraseBeforeTrailingPunctuation(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html><body><doc><page width="612" height="792">
<word xMin="100" yMin="200" xMax="145" yMax="214">Firmado</word>
<word xMin="150" yMin="200" xMax="170" yMax="214">por</word>
<word xMin="175" yMin="200" xMax="230" yMax="214">solicitante:</word>
</page></doc></body></html>`)
	words, err := ParseBBoxWords(data)
	if err != nil {
		t.Fatal(err)
	}
	matches := FindMarker(words, "Firmado por solicitante", false)
	if len(matches) != 1 {
		t.Fatalf("matches=%d, want 1", len(matches))
	}
}

func TestFindMarkerDoesNotMatchPrefixOfLongerWord(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html><body><doc><page width="612" height="792">
<word xMin="100" yMin="200" xMax="160" yMax="214">Firmante</word>
</page></doc></body></html>`)
	words, err := ParseBBoxWords(data)
	if err != nil {
		t.Fatal(err)
	}
	matches := FindMarker(words, "Firma", false)
	if len(matches) != 0 {
		t.Fatalf("matches=%d, want 0", len(matches))
	}
}
