package autofirma

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Rect struct {
	Page int
	LLX  float64
	LLY  float64
	URX  float64
	URY  float64
}

type SignOptions struct {
	AutoFirmaPath        string
	InputPath            string
	OutputPath           string
	Rect                 Rect
	Layer2Text           string
	FontSize             int
	FontFamily           int
	FontStyle            int
	FontColor            string
	Headless             bool
	CertGUI              bool
	GUI                  bool
	Store                string
	Password             string
	Alias                string
	Filter               string
	Algorithm            string
	Format               string
	ExtraParams          map[string]string
	Timeout              time.Duration
	DryRun               bool
	Verbose              bool
	PreserveConfigSpaces bool
	ConfigSeparator      string
	Visible              bool
}

type AliasOptions struct {
	AutoFirmaPath string
	Store         string
	Password      string
	XML           bool
	Timeout       time.Duration
}

type Result struct {
	Command string
	Args    []string
	Config  string
	Stdout  string
	Stderr  string
}

func DetectExecutable(explicit string) (string, error) {
	if explicit != "" {
		if isExecutablePath(explicit) {
			return explicit, nil
		}
		if p, err := exec.LookPath(explicit); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("no se encontró AutoFirma en %q", explicit)
	}

	candidates := []string{"AutoFirma", "autofirma", "Autofirma", "AutoFirmaCommandLine", "AutofirmaCommandLine", "autofirmacommandline"}
	if runtime.GOOS == "linux" {
		candidates = append([]string{
			"/usr/bin/AutoFirma",
			"/usr/bin/autofirma",
			"/usr/bin/Autofirma",
			"/usr/local/bin/AutoFirma",
			"/usr/local/bin/autofirma",
		}, candidates...)
	}
	seen := map[string]bool{}
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		if strings.ContainsRune(c, os.PathSeparator) {
			if isExecutablePath(c) {
				return c, nil
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no se encontró AutoFirma en PATH; instala AutoFirma o usa --autofirma /ruta/al/binario")
}

func Sign(ctx context.Context, opts SignOptions) (Result, error) {
	if opts.InputPath == "" || opts.OutputPath == "" {
		return Result{}, errors.New("InputPath y OutputPath son obligatorios")
	}
	if opts.Rect.Page <= 0 {
		return Result{}, errors.New("la página de firma debe ser >= 1")
	}
	if opts.Format == "" {
		opts.Format = "PAdES"
	}
	if opts.Algorithm == "" {
		opts.Algorithm = "SHA256withRSA"
	}
	if opts.FontSize <= 0 {
		opts.FontSize = 8
	}
	if opts.FontColor == "" {
		opts.FontColor = "black"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}

	config := BuildConfig(opts)

	afPath, err := DetectExecutable(opts.AutoFirmaPath)
	if err != nil {
		if !opts.DryRun {
			return Result{}, err
		}
		afPath = opts.AutoFirmaPath
		if afPath == "" {
			afPath = "AutoFirma"
		}
	}

	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("no se pudo crear el directorio de salida: %w", err)
	}

	args := []string{"sign"}
	if opts.GUI {
		args = append(args, "-gui")
	} else if opts.CertGUI {
		args = append(args, "-certgui")
	}
	args = append(args,
		"-i", opts.InputPath,
		"-o", opts.OutputPath,
		"-algorithm", opts.Algorithm,
		"-format", opts.Format,
	)
	if strings.TrimSpace(config) != "" {
		args = append(args, "-config", config)
	}
	if opts.Store != "" {
		args = append(args, "-store", opts.Store)
	}
	if opts.Password != "" {
		args = append(args, "-password", opts.Password)
	}
	if opts.Alias != "" {
		args = append(args, "-alias", opts.Alias)
	}
	if opts.Filter != "" {
		args = append(args, "-filter", opts.Filter)
	}

	res := Result{Command: afPath, Args: append([]string(nil), args...), Config: config}
	if opts.DryRun {
		return res, nil
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, afPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	if ctx.Err() != nil {
		return res, fmt.Errorf("AutoFirma excedió el tiempo máximo (%s): %w", opts.Timeout, ctx.Err())
	}
	if err != nil {
		return res, formatAutoFirmaError(err, res)
	}
	if _, err := os.Stat(opts.OutputPath); err != nil {
		return res, fmt.Errorf("AutoFirma terminó sin error, pero no existe el fichero de salida %q: %w", opts.OutputPath, err)
	}
	return res, nil
}

func ListAliases(ctx context.Context, opts AliasOptions) (Result, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	afPath, err := DetectExecutable(opts.AutoFirmaPath)
	if err != nil {
		return Result{}, err
	}
	args := []string{"listaliases"}
	if opts.Store != "" {
		args = append(args, "-store", opts.Store)
	}
	if opts.Password != "" {
		args = append(args, "-password", opts.Password)
	}
	if opts.XML {
		args = append(args, "-xml")
	}
	res := Result{Command: afPath, Args: append([]string(nil), args...)}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, afPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	if ctx.Err() != nil {
		return res, fmt.Errorf("AutoFirma excedió el tiempo máximo (%s): %w", opts.Timeout, ctx.Err())
	}
	if err != nil {
		return res, formatAutoFirmaError(err, res)
	}
	return res, nil
}

const DefaultLayer2Text = ""

func BuildConfig(opts SignOptions) string {
	// Keep the default AutoFirma configuration deliberately minimal.
	// Several Linux builds of AutoFirma accept the visible rectangle only when
	// -config is a plain Properties string separated with literal \n sequences and
	// contains just the position properties. AutoFirma then supplies its own
	// default layer2Text, exactly as its GUI does.
	props := map[string]string{}
	if opts.Headless {
		props["headLess"] = "true"
	}
	if opts.Visible {
		// Order mirrors the historically working Linux command line examples.
		props["signaturePositionOnPageUpperRightY"] = fmtFloat(opts.Rect.URY)
		props["signaturePositionOnPageUpperRightX"] = fmtFloat(opts.Rect.URX)
		props["signaturePositionOnPageLowerLeftY"] = fmtFloat(opts.Rect.LLY)
		props["signaturePositionOnPageLowerLeftX"] = fmtFloat(opts.Rect.LLX)
		props["signaturePage"] = fmt.Sprintf("%d", opts.Rect.Page)

		if strings.TrimSpace(opts.Layer2Text) != "" {
			props["layer2Text"] = normalizeConfigValue(opts.Layer2Text, opts.PreserveConfigSpaces)
			props["layer2FontSize"] = fmt.Sprintf("%d", opts.FontSize)
			props["layer2FontFamily"] = fmt.Sprintf("%d", opts.FontFamily)
			props["layer2FontStyle"] = fmt.Sprintf("%d", opts.FontStyle)
			props["layer2FontColor"] = opts.FontColor
		}
	}
	for k, v := range opts.ExtraParams {
		if strings.TrimSpace(k) == "" {
			continue
		}
		props[strings.TrimSpace(k)] = normalizeConfigValue(v, opts.PreserveConfigSpaces)
	}

	preferred := []string{
		"headLess",
		"signaturePositionOnPageUpperRightY",
		"signaturePositionOnPageUpperRightX",
		"signaturePositionOnPageLowerLeftY",
		"signaturePositionOnPageLowerLeftX",
		"signaturePage",
		"layer2Text",
		"layer2FontSize",
		"layer2FontFamily",
		"layer2FontStyle",
		"layer2FontColor",
	}
	used := map[string]bool{}
	lines := make([]string, 0, len(props))
	for _, k := range preferred {
		if v, ok := props[k]; ok {
			lines = append(lines, k+"="+v)
			used[k] = true
		}
	}
	var rest []string
	for k := range props {
		if !used[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		lines = append(lines, k+"="+props[k])
	}

	switch strings.ToLower(strings.TrimSpace(opts.ConfigSeparator)) {
	case "", "literal", "backslash-n", "slash-n":
		return strings.Join(lines, `\n`)
	case "newline", "lf":
		return strings.Join(lines, "\n")
	default:
		return strings.Join(lines, `\n`)
	}
}

func PrettyConfig(config string) string {
	return strings.ReplaceAll(config, `\n`, "\n")
}

func ShellQuote(args []string) string {
	return shellQuote(args, nil)
}

func ShellQuoteRedacted(args []string) string {
	redactNext := map[string]bool{"-password": true}
	return shellQuote(args, redactNext)
}

func shellQuote(args []string, redactNext map[string]bool) string {
	parts := make([]string, len(args))
	redact := false
	for i, a := range args {
		if redact {
			a = "***"
			redact = false
		}
		if redactNext != nil && redactNext[a] {
			redact = true
		}
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

func isExecutablePath(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	return st.Mode()&0o111 != 0
}

func formatAutoFirmaError(err error, res Result) error {
	stdout := strings.TrimSpace(res.Stdout)
	stderr := strings.TrimSpace(res.Stderr)
	cmd := ShellQuoteRedacted(append([]string{res.Command}, res.Args...))
	var parts []string
	parts = append(parts, fmt.Sprintf("AutoFirma falló: %v", err))
	if stderr != "" {
		parts = append(parts, "stderr: "+stderr)
	}
	if stdout != "" {
		parts = append(parts, "stdout: "+stdout)
	}
	if stderr == "" && stdout == "" {
		parts = append(parts, "sin salida por stdout/stderr")
	}
	parts = append(parts, "comando: "+cmd)
	parts = append(parts, "pistas: en Linux prueba primero --probe-stores y --list-aliases --store mozilla; si el certificado está en Firefox usa --store mozilla; si está en fichero usa --store pkcs12:/ruta/cert.p12; si es DNIe usa --store dni")
	return errors.New(strings.Join(parts, "; "))
}

func normalizeConfigValue(s string, preserveSpaces bool) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	// Avoid breaking old Linux wrappers that split the -config argument on spaces.
	if !preserveSpaces {
		s = strings.Join(strings.Fields(s), "_")
	} else {
		s = strings.Join(strings.Fields(s), " ")
	}
	return escapePropertyValue(s)
}

func fmtFloat(v float64) string {
	// AutoFirma's PAdES visible-signature parser in some Linux builds parses
	// the coordinates as integers (Integer.parseInt). Passing decimal values
	// such as "56.8" causes AutoFirma to sign the PDF but discard the visible
	// rectangle with: NumberFormatException: For input string: "56.8".
	// Round to the nearest PDF point so the rectangle remains at the marker
	// position while staying compatible with those builds.
	n := int(math.Round(v))
	if n == 0 {
		return "0"
	}
	return fmt.Sprintf("%d", n)
}

func escapePropertyValue(s string) string {
	// Keep the value in a single Java-properties line. The command-line config
	// itself is separated with literal \n sequences, so literal backslashes are
	// preserved instead of introducing properties-level continuation rules.
	s = strings.ReplaceAll(s, "=", "\\=")
	return s
}
