package pdfmark

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const DefaultMarker = "[FIRMA]"

type Anchor string

const (
	AnchorLowerLeft Anchor = "lower-left"
	AnchorCenter    Anchor = "center"
	AnchorTopLeft   Anchor = "top-left"
)

type Box struct {
	XMin float64
	YMin float64
	XMax float64
	YMax float64
}

type Word struct {
	PageIndex  int
	PageNumber int
	PageWidth  float64
	PageHeight float64
	Text       string
	Box        Box
}

type Match struct {
	PageNumber int
	PageWidth  float64
	PageHeight float64
	Text       string
	BoxTopLeft Box
	Words      []Word
}

type Rect struct {
	Page       int
	PageWidth  float64
	PageHeight float64
	LLX        float64
	LLY        float64
	URX        float64
	URY        float64
}

type LocateOptions struct {
	Marker          string
	PDFToTextPath   string
	CaseInsensitive bool
	Timeout         time.Duration
}

func Locate(ctx context.Context, pdfPath string, opts LocateOptions) ([]Match, error) {
	marker := opts.Marker
	if marker == "" {
		marker = DefaultMarker
	}
	pdftotext := opts.PDFToTextPath
	if pdftotext == "" {
		pdftotext = "pdftotext"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 45 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, pdftotext, "-bbox", "-enc", "UTF-8", pdfPath, "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("pdftotext excedió el tiempo máximo (%s): %w", opts.Timeout, ctx.Err())
		}
		return nil, fmt.Errorf("falló pdftotext -bbox para %q: %w: %s", pdfPath, err, strings.TrimSpace(stderr.String()))
	}

	words, err := ParseBBoxWords(out)
	if err != nil {
		return nil, err
	}
	return FindMarker(words, marker, opts.CaseInsensitive), nil
}

func ParseBBoxWords(data []byte) ([]Word, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false
	var words []Word
	var currentPage struct {
		index  int
		number int
		width  float64
		height float64
	}
	currentPage.number = 0

	var inWord bool
	var active Word
	var textBuilder strings.Builder

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("no se pudo parsear la salida XHTML de pdftotext: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := localName(t.Name)
			switch name {
			case "page":
				currentPage.index++
				currentPage.number = currentPage.index
				currentPage.width = attrFloat(t, "width")
				currentPage.height = attrFloat(t, "height")
			case "word":
				inWord = true
				textBuilder.Reset()
				active = Word{
					PageIndex:  currentPage.index - 1,
					PageNumber: currentPage.number,
					PageWidth:  currentPage.width,
					PageHeight: currentPage.height,
					Box: Box{
						XMin: attrFloat(t, "xMin"),
						YMin: attrFloat(t, "yMin"),
						XMax: attrFloat(t, "xMax"),
						YMax: attrFloat(t, "yMax"),
					},
				}
			}
		case xml.CharData:
			if inWord {
				textBuilder.Write([]byte(t))
			}
		case xml.EndElement:
			if localName(t.Name) == "word" && inWord {
				active.Text = cleanToken(html.UnescapeString(textBuilder.String()))
				if active.Text != "" {
					words = append(words, active)
				}
				inWord = false
				textBuilder.Reset()
			}
		}
	}
	return words, nil
}

func FindMarker(words []Word, marker string, caseInsensitive bool) []Match {
	if marker == "" || len(words) == 0 {
		return nil
	}
	needle := normalizeMarker(marker, caseInsensitive)
	var matches []Match

	for i := 0; i < len(words); i++ {
		if words[i].PageNumber <= 0 {
			continue
		}
		joined := ""
		var union Box
		var seq []Word
		for j := i; j < len(words); j++ {
			if words[j].PageNumber != words[i].PageNumber {
				break
			}
			piece := normalizeMarker(words[j].Text, caseInsensitive)
			if piece == "" {
				continue
			}
			joined += piece
			seq = append(seq, words[j])
			if len(seq) == 1 {
				union = words[j].Box
			} else {
				union = unionBox(union, words[j].Box)
			}
			if markerJoinedMatches(joined, needle) {
				matches = append(matches, Match{
					PageNumber: words[i].PageNumber,
					PageWidth:  words[i].PageWidth,
					PageHeight: words[i].PageHeight,
					Text:       marker,
					BoxTopLeft: union,
					Words:      append([]Word(nil), seq...),
				})
				break
			}
			if (!strings.HasPrefix(needle, joined) && !strings.HasPrefix(joined, needle)) || len(joined) > len(needle)+16 {
				break
			}
		}
	}
	return matches
}

func RectForMatch(m Match, width, height, offsetX, offsetY, margin float64, anchor Anchor) Rect {
	if width <= 0 {
		width = 220
	}
	if height <= 0 {
		height = 80
	}

	markerWidth := math.Max(0, m.BoxTopLeft.XMax-m.BoxTopLeft.XMin)
	markerHeight := math.Max(0, m.BoxTopLeft.YMax-m.BoxTopLeft.YMin)
	markerLLY := m.PageHeight - m.BoxTopLeft.YMax
	markerURY := m.PageHeight - m.BoxTopLeft.YMin

	var llx, lly float64
	switch anchor {
	case AnchorCenter:
		cx := m.BoxTopLeft.XMin + markerWidth/2
		cy := markerLLY + markerHeight/2
		llx = cx - width/2
		lly = cy - height/2
	case AnchorTopLeft:
		llx = m.BoxTopLeft.XMin
		lly = markerURY - height
	default:
		llx = m.BoxTopLeft.XMin
		lly = markerLLY
	}

	llx += offsetX
	lly += offsetY
	urx := llx + width
	ury := lly + height

	if margin < 0 {
		margin = 0
	}
	if m.PageWidth > 0 && m.PageHeight > 0 {
		if llx < margin {
			urx += margin - llx
			llx = margin
		}
		if lly < margin {
			ury += margin - lly
			lly = margin
		}
		if urx > m.PageWidth-margin {
			delta := urx - (m.PageWidth - margin)
			llx -= delta
			urx -= delta
		}
		if ury > m.PageHeight-margin {
			delta := ury - (m.PageHeight - margin)
			lly -= delta
			ury -= delta
		}
		if llx < margin {
			llx = margin
		}
		if lly < margin {
			lly = margin
		}
	}

	return Rect{
		Page:       m.PageNumber,
		PageWidth:  m.PageWidth,
		PageHeight: m.PageHeight,
		LLX:        round2(llx),
		LLY:        round2(lly),
		URX:        round2(urx),
		URY:        round2(ury),
	}
}

func ParseAnchor(s string) (Anchor, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "_", "-")
	switch Anchor(s) {
	case AnchorLowerLeft, AnchorCenter, AnchorTopLeft:
		return Anchor(s), nil
	default:
		return "", fmt.Errorf("anchor inválido %q; valores válidos: lower-left, center, top-left", s)
	}
}

func cleanToken(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\u200b", "")
	s = strings.ReplaceAll(s, "\ufeff", "")
	return s
}

func normalizeMarker(s string, caseInsensitive bool) string {
	s = cleanToken(s)
	// Word-level extraction may split [ FIRMA ] into separate tokens. For marker matching,
	// whitespace between adjacent extracted words is ignored.
	s = strings.Join(strings.Fields(s), "")
	if caseInsensitive {
		s = strings.ToUpper(s)
	}
	return s
}

func markerJoinedMatches(joined, needle string) bool {
	if joined == needle {
		return true
	}
	if !strings.HasPrefix(joined, needle) {
		return false
	}
	// pdftotext often keeps punctuation attached to the final word. Let
	// "Texto de firma" match extracted words like "Texto", "de", "firma:".
	for _, r := range joined[len(needle):] {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return false
		}
	}
	return true
}

func unionBox(a, b Box) Box {
	return Box{
		XMin: math.Min(a.XMin, b.XMin),
		YMin: math.Min(a.YMin, b.YMin),
		XMax: math.Max(a.XMax, b.XMax),
		YMax: math.Max(a.YMax, b.YMax),
	}
}

func attrFloat(el xml.StartElement, name string) float64 {
	for _, a := range el.Attr {
		if localName(a.Name) == name {
			v, _ := strconv.ParseFloat(strings.TrimSpace(a.Value), 64)
			return v
		}
	}
	return 0
}

func localName(n xml.Name) string {
	if n.Local != "" {
		return n.Local
	}
	return n.Space
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
