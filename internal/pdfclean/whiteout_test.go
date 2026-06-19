package pdfclean

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pdfsignmark/internal/pdfmark"
)

func TestRectForMarkerConvertsToPDFCoordinates(t *testing.T) {
	m := pdfmark.Match{
		PageNumber: 1,
		PageWidth:  612,
		PageHeight: 792,
		BoxTopLeft: pdfmark.Box{XMin: 56.8, YMin: 350.21, XMax: 110.2, YMax: 367.21},
	}
	r := RectForMarker(m, 2)
	if r.Page != 1 || r.LLX != 54.8 || r.URX != 112.2 || r.LLY != 422.79 || r.URY != 443.79 {
		t.Fatalf("unexpected cleanup rect: %+v", r)
	}
}

func TestWhiteoutMarkerChangesRenderedPage(t *testing.T) {
	gs, err := exec.LookPath("gs")
	if err != nil {
		t.Skip("ghostscript not installed")
	}
	input := filepath.Join("..", "..", "examples", "marker-test.pdf")
	tmp := t.TempDir()
	out := filepath.Join(tmp, "clean.pdf")
	_, err = WhiteoutMarker(context.Background(), Options{
		InputPath:  input,
		OutputPath: out,
		// Coordinates around the [FIRMA] marker in examples/marker-test.pdf.
		Rect: Rect{Page: 1, LLX: 298, LLY: 195, URX: 346, URY: 211},
	})
	if err != nil {
		t.Fatalf("WhiteoutMarker() error = %v", err)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		t.Fatalf("cleaned output missing or empty: %v", err)
	}

	origPNG := filepath.Join(tmp, "original.png")
	cleanPNG := filepath.Join(tmp, "clean.png")
	renderPage(t, gs, input, origPNG, 1)
	renderPage(t, gs, out, cleanPNG, 1)
	origBytes := mustRead(t, origPNG)
	cleanBytes := mustRead(t, cleanPNG)
	if bytes.Equal(origBytes, cleanBytes) {
		t.Fatalf("cleaned PDF rendered identically to input; whiteout was probably not applied (sha256=%s)", sha256Hex(origBytes))
	}
}

func TestWhiteoutMarkerPreservesPageCount(t *testing.T) {
	gs, err := exec.LookPath("gs")
	if err != nil {
		t.Skip("ghostscript not installed")
	}
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo not installed")
	}
	tmp := t.TempDir()
	input := filepath.Join(tmp, "three-pages.pdf")
	createThreePagePDF(t, gs, input)
	out := filepath.Join(tmp, "clean.pdf")
	_, err = WhiteoutMarker(context.Background(), Options{
		InputPath:  input,
		OutputPath: out,
		Rect:       Rect{Page: 3, LLX: 150, LLY: 695, URX: 230, URY: 715},
	})
	if err != nil {
		t.Fatalf("WhiteoutMarker() error = %v", err)
	}
	if got := pdfPageCount(t, input); got != 3 {
		t.Fatalf("input page count = %d, want 3", got)
	}
	if got := pdfPageCount(t, out); got != 3 {
		t.Fatalf("output page count = %d, want 3; cleanup must not add blank pages", got)
	}
}

func TestWhiteoutMarkerStampsOnlyTargetPage(t *testing.T) {
	gs, err := exec.LookPath("gs")
	if err != nil {
		t.Skip("ghostscript not installed")
	}
	tmp := t.TempDir()
	input := filepath.Join(tmp, "three-pages.pdf")
	createThreePagePDF(t, gs, input)
	out := filepath.Join(tmp, "clean.pdf")

	// Target the marker on the LAST page. This is the regression case: the
	// per-page save/restore in Ghostscript's PDF interpreter used to reset the
	// page counter, so the rectangle was only ever evaluated for page 1 and the
	// marker on later pages stayed visible.
	rect := Rect{Page: 3, LLX: 150, LLY: 695, URX: 230, URY: 715}
	if _, err := WhiteoutMarker(context.Background(), Options{
		InputPath:  input,
		OutputPath: out,
		Rect:       rect,
	}); err != nil {
		t.Fatalf("WhiteoutMarker() error = %v", err)
	}

	// The synthetic PDF paints a gray rectangle in this same area on every page.
	// Whiteout should turn the target page region back to white.
	targetPNG := filepath.Join(tmp, "p3.png")
	renderPage(t, gs, out, targetPNG, 3)
	filled, area := fillRatioInRect(t, targetPNG, rect, renderDPI)
	if ratio := float64(filled) / float64(area); ratio > 0.1 {
		t.Fatalf("target page 3 non-white ratio = %.2f in marker rectangle %+v; expected the whiteout box to cover the marker", ratio, rect)
	}

	// The box must appear ONLY on the target page. Page 1 has the same gray
	// rectangle in this region, so it must remain visibly non-white.
	otherPNG := filepath.Join(tmp, "p1.png")
	renderPage(t, gs, out, otherPNG, 1)
	n, area := fillRatioInRect(t, otherPNG, rect, renderDPI)
	if float64(n)/float64(area) < 0.5 {
		t.Fatalf("page 1 has %d non-white pixels (%.2f ratio) in the marker rectangle; the whiteout box was drawn on the wrong page", n, float64(n)/float64(area))
	}
}

const renderDPI = 96.0

// fillRatioInRect renders a PDF-space rectangle (origin bottom-left, points) to
// the pixel region of the given PNG and returns the number of non-white pixels
// and the region area. The PDF page height is derived from the image height so
// the test does not depend on Ghostscript's default page size (letter vs A4).
func fillRatioInRect(t *testing.T, pngPath string, rect Rect, dpi float64) (filled, area int) {
	t.Helper()
	f, err := os.Open(pngPath)
	if err != nil {
		t.Fatalf("open %s: %v", pngPath, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", pngPath, err)
	}
	scale := dpi / 72.0
	bounds := img.Bounds()
	pxMinX := bounds.Min.X + int(rect.LLX*scale)
	pxMaxX := bounds.Min.X + int(rect.URX*scale)
	// PDF y-origin is bottom-left; image y-origin is top-left.
	pxMinY := bounds.Max.Y - int(rect.URY*scale)
	pxMaxY := bounds.Max.Y - int(rect.LLY*scale)
	region := image.Rect(pxMinX, pxMinY, pxMaxX, pxMaxY).Intersect(bounds)
	if region.Empty() {
		t.Fatalf("computed pixel region is empty for rect %+v in image %v", rect, bounds)
	}
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// RGBA() returns 16-bit values; treat anything clearly darker than
			// white as filled. Text glyphs are thin, the rectangle fills the area.
			if r < 0xC000 || g < 0xC000 || b < 0xC000 {
				filled++
			}
		}
	}
	return filled, region.Dx() * region.Dy()
}

func renderPage(t *testing.T, gs, pdf, out string, page int) {
	t.Helper()
	cmd := exec.Command(gs,
		"-q",
		"-dBATCH",
		"-dNOPAUSE",
		"-sDEVICE=pnggray",
		"-r96",
		"-dFirstPage="+strconv.Itoa(page),
		"-dLastPage="+strconv.Itoa(page),
		"-o", out,
		pdf,
	)
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render failed for %s: %v: %s", pdf, err, combined)
	}
}

func createThreePagePDF(t *testing.T, gs, out string) {
	t.Helper()
	tmp := t.TempDir()
	ps := filepath.Join(tmp, "in.ps")
	content := `%!PS-Adobe-3.0
%%Pages: 3
/Courier findfont 12 scalefont setfont
/markarea { gsave 0.70 setgray 150 695 80 20 rectfill grestore } def
markarea 0 setgray 100 700 moveto (Page 1) show showpage
markarea 0 setgray 100 700 moveto (Page 2) show showpage
markarea 0 setgray 100 700 moveto (Page 3 [FIRMA]) show showpage
`
	if err := os.WriteFile(ps, []byte(content), 0o600); err != nil {
		t.Fatalf("write ps: %v", err)
	}
	cmd := exec.Command(gs, "-q", "-dBATCH", "-dNOPAUSE", "-sDEVICE=pdfwrite", "-o", out, ps)
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create pdf: %v: %s", err, combined)
	}
}

func pdfPageCount(t *testing.T, pdf string) int {
	t.Helper()
	out, err := exec.Command("pdfinfo", pdf).Output()
	if err != nil {
		t.Fatalf("pdfinfo %s: %v", pdf, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			fields := strings.Fields(line)
			if len(fields) == 2 {
				n, err := strconv.Atoi(fields[1])
				if err == nil {
					return n
				}
			}
		}
	}
	t.Fatalf("pdfinfo did not include Pages line: %s", out)
	return 0
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func sha256Hex(b []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
