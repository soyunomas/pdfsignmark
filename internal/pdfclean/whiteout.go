package pdfclean

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pdfsignmark/internal/pdfmark"
)

type Rect struct {
	Page int
	LLX  float64
	LLY  float64
	URX  float64
	URY  float64
}

type Options struct {
	GhostscriptPath string
	InputPath       string
	OutputPath      string
	Rect            Rect
	Timeout         time.Duration
	DryRun          bool
}

type Result struct {
	Command string
	Args    []string
	PS      string
	Stdout  string
	Stderr  string
}

// RectForMarker returns the PDF-coordinate rectangle that visually covers the
// marker text found by pdftotext -bbox. This is intentionally a visual cleanup:
// it paints a white rectangle over the marker before AutoFirma signs the file.
func RectForMarker(m pdfmark.Match, padding float64) Rect {
	if padding < 0 {
		padding = 0
	}
	llx := m.BoxTopLeft.XMin - padding
	lly := m.PageHeight - m.BoxTopLeft.YMax - padding
	urx := m.BoxTopLeft.XMax + padding
	ury := m.PageHeight - m.BoxTopLeft.YMin + padding
	if llx < 0 {
		llx = 0
	}
	if lly < 0 {
		lly = 0
	}
	if m.PageWidth > 0 && urx > m.PageWidth {
		urx = m.PageWidth
	}
	if m.PageHeight > 0 && ury > m.PageHeight {
		ury = m.PageHeight
	}
	return Rect{Page: m.PageNumber, LLX: llx, LLY: lly, URX: urx, URY: ury}
}

// WhiteoutMarker creates a visually cleaned PDF by installing a Ghostscript
// EndPage hook that draws a white rectangle on top of the target marker area
// before the selected page is finalized.
func WhiteoutMarker(ctx context.Context, opts Options) (Result, error) {
	if opts.InputPath == "" || opts.OutputPath == "" {
		return Result{}, errors.New("InputPath y OutputPath son obligatorios")
	}
	if opts.Rect.Page <= 0 {
		return Result{}, errors.New("la página de limpieza debe ser >= 1")
	}
	if opts.Rect.URX <= opts.Rect.LLX || opts.Rect.URY <= opts.Rect.LLY {
		return Result{}, fmt.Errorf("rectángulo de limpieza inválido: %+v", opts.Rect)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}

	gsPath, err := detectGhostscript(opts.GhostscriptPath)
	if err != nil {
		if !opts.DryRun {
			return Result{}, err
		}
		gsPath = opts.GhostscriptPath
		if gsPath == "" {
			gsPath = "gs"
		}
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("no se pudo crear el directorio de salida temporal: %w", err)
	}

	absInput, err := filepath.Abs(opts.InputPath)
	if err != nil {
		absInput = opts.InputPath
	}

	ps := buildWhiteoutPostScript(absInput, opts.Rect)
	args := []string{
		"-q",
		"-dBATCH",
		"-dNOPAUSE",
		"-dSAFER",
		"--permit-file-read=" + absInput,
		"-sDEVICE=pdfwrite",
		"-dCompatibilityLevel=1.7",
		"-o", opts.OutputPath,
	}
	res := Result{Command: gsPath, PS: ps}
	if opts.DryRun {
		res.Args = append(args, "-c", ps)
		return res, nil
	}

	tmpDir, err := os.MkdirTemp("", "pdfsignmark-pdfclean-*")
	if err != nil {
		return res, fmt.Errorf("no se pudo crear directorio temporal para Ghostscript: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	psPath := filepath.Join(tmpDir, "whiteout.ps")
	if err := os.WriteFile(psPath, []byte(ps), 0o600); err != nil {
		return res, fmt.Errorf("no se pudo escribir programa temporal de Ghostscript: %w", err)
	}
	args = append(args, "-f", psPath)
	res.Args = append([]string(nil), args...)

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gsPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	if ctx.Err() != nil {
		return res, fmt.Errorf("Ghostscript excedió el tiempo máximo (%s): %w", opts.Timeout, ctx.Err())
	}
	if err != nil {
		return res, formatGhostscriptError(err, res)
	}
	if _, err := os.Stat(opts.OutputPath); err != nil {
		return res, fmt.Errorf("Ghostscript terminó sin error, pero no existe el fichero intermedio %q: %w", opts.OutputPath, err)
	}
	return res, nil
}

func buildWhiteoutPostScript(inputPath string, r Rect) string {
	// Use Ghostscript's page device EndPage hook with a page counter kept in
	// global VM. This is deliberately simpler than trying to drive Ghostscript's
	// PDF interpreter internals (pdfgetpage/pdfshowpage_*), which differ across
	// Ghostscript versions. The procedure draws the white rectangle immediately
	// before the selected page is emitted, and returns true so Ghostscript emits
	// exactly the original page. Reason code 2 is suppressed, as recommended for
	// EndPage procedures, because it is called for device deactivation rather than
	// a real page.
	var b strings.Builder
	fmt.Fprintf(&b, "/TargetPage %d def\n", r.Page)
	fmt.Fprintf(&b, "/BoxLLX %s def /BoxLLY %s def /BoxW %s def /BoxH %s def\n", psFloat(r.LLX), psFloat(r.LLY), psFloat(r.URX-r.LLX), psFloat(r.URY-r.LLY))
	// Keep the page counter in global VM. Ghostscript's PDF interpreter wraps
	// each page in its own save/restore, so a counter stored in local VM (via
	// a plain `def`) is reverted after every page and never advances past 1.
	// That made multi-page documents fail: the white rectangle was only ever
	// considered for page 1 and the marker on later pages stayed visible.
	// globaldict lives in global VM, which is not affected by the per-page
	// restore, so the counter survives and reaches the real target page.
	b.WriteString("globaldict /PDFSIGNMARK_PageCount 0 put\n")
	b.WriteString("<< /EndPage {\n")
	b.WriteString("  exch pop\n")
	b.WriteString("  2 eq {\n")
	b.WriteString("    false\n")
	b.WriteString("  } {\n")
	b.WriteString("    globaldict /PDFSIGNMARK_PageCount globaldict /PDFSIGNMARK_PageCount get 1 add put\n")
	b.WriteString("    globaldict /PDFSIGNMARK_PageCount get TargetPage eq {\n")
	b.WriteString("      gsave\n")
	b.WriteString("      1 setgray newpath BoxLLX BoxLLY BoxW BoxH rectfill\n")
	b.WriteString("      grestore\n")
	b.WriteString("    } if\n")
	b.WriteString("    true\n")
	b.WriteString("  } ifelse\n")
	b.WriteString("} bind >> setpagedevice\n")
	b.WriteString(psLiteralString(inputPath))
	b.WriteString(" run\n")
	return b.String()
}

func psFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func psLiteralString(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, c := range []byte(s) {
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 32 || c > 126 {
				fmt.Fprintf(&b, "\\%03o", c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte(')')
	return b.String()
}

func detectGhostscript(explicit string) (string, error) {
	if explicit != "" {
		if p, err := exec.LookPath(explicit); err == nil {
			return p, nil
		}
		if st, err := os.Stat(explicit); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return explicit, nil
		}
		return "", fmt.Errorf("no se encontró Ghostscript en %q", explicit)
	}
	for _, c := range []string{"gs", "ghostscript"} {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no se encontró Ghostscript; instala el paquete 'ghostscript' o usa --gs /ruta/gs. También puedes usar --keep-marker para no ocultar el marcador")
}

func formatGhostscriptError(err error, res Result) error {
	stdout := strings.TrimSpace(res.Stdout)
	stderr := strings.TrimSpace(res.Stderr)
	parts := []string{fmt.Sprintf("Ghostscript falló al ocultar el marcador [FIRMA]: %v", err)}
	if stderr != "" {
		parts = append(parts, "stderr: "+stderr)
	}
	if stdout != "" {
		parts = append(parts, "stdout: "+stdout)
	}
	if stderr == "" && stdout == "" {
		parts = append(parts, "sin salida por stdout/stderr")
	}
	parts = append(parts, "comando: "+ShellQuote(append([]string{res.Command}, res.Args...)))
	return errors.New(strings.Join(parts, "; "))
}

func ShellQuote(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a == "" {
			parts[i] = "''"
			continue
		}
		if strings.IndexFunc(a, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\'' || r == '"' || r == '$' || r == '\\' || r == ';' || r == '&' || r == '|' || r == '<' || r == '>' || r == '(' || r == ')'
		}) == -1 {
			parts[i] = a
			continue
		}
		parts[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
	}
	return strings.Join(parts, " ")
}
