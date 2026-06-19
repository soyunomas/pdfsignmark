package main

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"pdfsignmark/internal/autofirma"
	"pdfsignmark/internal/pdfclean"
	"pdfsignmark/internal/pdfmark"
)

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

type appConfig struct {
	marker               string
	caseInsensitive      bool
	markerIndex          int
	anchorText           string
	width                float64
	height               float64
	offsetX              float64
	offsetY              float64
	pageMargin           float64
	keepMarker           bool
	cleanPadding         float64
	ghostscript          string
	outDir               string
	suffix               string
	overwrite            bool
	continueOnError      bool
	dryRun               bool
	verbose              bool
	listMarkers          bool
	listAliases          bool
	probeStores          bool
	aliasXML             bool
	pdftotext            string
	autofirma            string
	store                string
	password             string
	alias                string
	filter               string
	certgui              bool
	noCertGUI            bool
	gui                  bool
	headless             bool
	algorithm            string
	layer2Text           string
	fontSize             int
	fontFamily           int
	fontStyle            int
	fontColor            string
	preserveConfigSpaces bool
	configSeparator      string
	strictVisible        bool
	visibleMode          string
	extra                multiFlag
	timeout              time.Duration
}

func main() {
	cfg, inputs, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fatal(err)
	}
	if cfg.listAliases || cfg.probeStores {
		if err := runAliasMode(context.Background(), cfg); err != nil {
			fatal(err)
		}
		return
	}

	if len(inputs) == 0 {
		usageError("indica al menos un PDF; ejemplo: pdfsignmark ./*.pdf")
	}

	files, err := expandInputs(inputs)
	if err != nil {
		fatal(err)
	}
	if len(files) == 0 {
		fatal(errors.New("no se encontró ningún PDF en los parámetros indicados"))
	}

	anchor, err := pdfmark.ParseAnchor(cfg.anchorText)
	if err != nil {
		fatal(err)
	}

	ctx := context.Background()
	if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
		fatal(fmt.Errorf("no se pudo crear --out-dir %q: %w", cfg.outDir, err))
	}
	cfg, err = prepareBatchCertificate(ctx, cfg, len(files))
	if err != nil {
		fatal(err)
	}

	var ok, failed int
	allowCertGUI := true
	for _, in := range files {
		usedCertGUI, err := processOne(ctx, cfg, anchor, in, allowCertGUI)
		if usedCertGUI {
			allowCertGUI = false
		}
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "ERROR %s: %v\n", in, err)
			if !cfg.continueOnError {
				os.Exit(1)
			}
			continue
		}
		ok++
	}

	if failed > 0 {
		fmt.Fprintf(os.Stderr, "Completado con errores: OK=%d, errores=%d\n", ok, failed)
		os.Exit(2)
	}
	fmt.Printf("Completado: %d PDF procesado(s).\n", ok)
}

func parseArgs(args []string) (appConfig, []string, error) {
	cfg := appConfig{}
	fs := flag.NewFlagSet("pdfsignmark", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.StringVar(&cfg.marker, "marker", pdfmark.DefaultMarker, "Texto marcador a localizar dentro del PDF")
	fs.BoolVar(&cfg.caseInsensitive, "case-insensitive", false, "Buscar el marcador sin distinguir mayúsculas/minúsculas")
	fs.IntVar(&cfg.markerIndex, "marker-index", 1, "Marcador a usar cuando hay varios en el PDF: 1=primero, 2=segundo, -1=último")
	fs.StringVar(&cfg.anchorText, "anchor", string(pdfmark.AnchorCenter), "Cómo anclar el cajetín al marcador: center (el marcador queda en el centro de la firma), lower-left, top-left")
	fs.Float64Var(&cfg.width, "width", 220, "Anchura del cajetín visible de firma en puntos PDF")
	fs.Float64Var(&cfg.height, "height", 80, "Altura del cajetín visible de firma en puntos PDF")
	fs.Float64Var(&cfg.offsetX, "offset-x", 0, "Desplazamiento horizontal desde el marcador, en puntos PDF")
	fs.Float64Var(&cfg.offsetY, "offset-y", 0, "Desplazamiento vertical desde el marcador, en puntos PDF")
	fs.Float64Var(&cfg.pageMargin, "page-margin", 5, "Margen mínimo contra los bordes de página, en puntos PDF")
	fs.BoolVar(&cfg.keepMarker, "keep-marker", false, "No ocultar visualmente el marcador antes de firmar")
	fs.Float64Var(&cfg.cleanPadding, "clean-padding", 2, "Margen blanco alrededor del marcador ocultado, en puntos PDF")
	fs.StringVar(&cfg.ghostscript, "gs", "gs", "Ruta del binario Ghostscript usado para ocultar el marcador antes de firmar")

	fs.StringVar(&cfg.outDir, "out-dir", ".", "Directorio de salida")
	fs.StringVar(&cfg.suffix, "suffix", "_firmado", "Sufijo para los PDF firmados")
	fs.BoolVar(&cfg.overwrite, "overwrite", false, "Sobrescribir salidas existentes")
	fs.BoolVar(&cfg.continueOnError, "continue-on-error", false, "Continuar con el resto de PDF si uno falla")
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "No firmar; solo mostrar ubicación y comando AutoFirma")
	fs.BoolVar(&cfg.verbose, "v", false, "Salida detallada")
	fs.BoolVar(&cfg.listMarkers, "list-markers", false, "Listar coordenadas de marcadores y no firmar")
	fs.BoolVar(&cfg.listAliases, "list-aliases", false, "Listar alias de certificados que ve AutoFirma en el almacén indicado por --store y no firmar")
	fs.BoolVar(&cfg.probeStores, "probe-stores", false, "Probar almacenes habituales de AutoFirma en Linux, normalmente mozilla y auto, y no firmar")
	fs.BoolVar(&cfg.aliasXML, "alias-xml", false, "Pedir salida XML a listaliases")

	fs.StringVar(&cfg.pdftotext, "pdftotext", "pdftotext", "Ruta del binario pdftotext de Poppler")
	fs.StringVar(&cfg.autofirma, "autofirma", "", "Ruta del binario/script de AutoFirma; si se omite, se busca en PATH")
	fs.StringVar(&cfg.store, "store", "", "Almacén AutoFirma: auto, mozilla, dni, pkcs12:/ruta/cert.p12, pkcs11:/ruta/lib.so, etc. En Linux, si se omite y no hay alias/filtro, se usa mozilla por defecto")
	fs.StringVar(&cfg.password, "password", "", "Contraseña del almacén indicado en --store; evita usarla en terminales compartidos")
	fs.StringVar(&cfg.alias, "alias", "", "Alias del certificado de firma")
	fs.StringVar(&cfg.filter, "filter", "", "Filtro AutoFirma de certificado, por ejemplo subject.contains:12345678Z;nonexpired:")
	fs.BoolVar(&cfg.certgui, "certgui", false, "Forzar diálogo gráfico de selección de certificado")
	fs.BoolVar(&cfg.noCertGUI, "no-certgui", false, "No usar selector gráfico de certificado por defecto")
	fs.BoolVar(&cfg.gui, "gui", false, "Ejecutar operación con entorno gráfico de AutoFirma")
	fs.BoolVar(&cfg.headless, "headless", false, "Configurar headLess=true en AutoFirma; requiere certificado seleccionable sin interacción")
	fs.StringVar(&cfg.algorithm, "algorithm", "SHA256withRSA", "Algoritmo de firma")
	fs.StringVar(&cfg.layer2Text, "text", "", "Texto visible personalizado dentro del cajetín. Si se omite, AutoFirma usa su texto por defecto; es lo más compatible en Linux")
	fs.IntVar(&cfg.fontSize, "font-size", 8, "Tamaño de fuente del texto visible")
	fs.IntVar(&cfg.fontFamily, "font-family", 1, "Familia de fuente AutoFirma: 0=Courier, 1=Helvetica, 2=Times, 3=Symbol, 4=ZapfDingBats")
	fs.IntVar(&cfg.fontStyle, "font-style", 0, "Estilo de fuente AutoFirma: 0=normal, 1=negrita, 2=cursiva, 3=negrita+cursiva, 4=subrayado, 8=tachado")
	fs.StringVar(&cfg.fontColor, "font-color", "black", "Color de fuente AutoFirma: black, white, gray, lightGray, darkGray, red, pink")
	fs.BoolVar(&cfg.preserveConfigSpaces, "preserve-config-spaces", false, "No sustituir espacios por guiones bajos en -config; úsalo solo si tu AutoFirma Linux respeta argumentos con espacios")
	fs.StringVar(&cfg.configSeparator, "config-separator", "literal", "Separador interno de -config: literal o newline. En Linux suele funcionar literal, es decir, \\n escrito como dos caracteres")
	fs.BoolVar(&cfg.strictVisible, "strict-visible", false, "Tras firmar, fallar si no se detecta heurísticamente una apariencia visible de firma en el PDF")
	fs.StringVar(&cfg.visibleMode, "visible-mode", "autofirma", "Modo de firma visible: autofirma o none")
	fs.Var(&cfg.extra, "extra", "Parámetro extra AutoFirma key=value; repetible")
	fs.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "Tiempo máximo por PDF para AutoFirma")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Uso:\n  pdfsignmark [opciones] ./*.pdf\n\nEjemplos:\n  pdfsignmark --probe-stores\n  pdfsignmark --list-aliases --store mozilla\n  pdfsignmark ./*.pdf\n  pdfsignmark --store mozilla --certgui ./*.pdf\n  pdfsignmark --filter 'subject.contains:12345678Z;nonexpired:' ./*.pdf\n  pdfsignmark --store pkcs12:/home/me/cert.p12 --password '***' ./*.pdf\n  pdfsignmark --dry-run --v documento.pdf\n\nOpciones:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return cfg, nil, err
	}
	if cfg.width <= 0 || cfg.height <= 0 {
		return cfg, nil, errors.New("--width y --height deben ser mayores que cero")
	}
	if cfg.cleanPadding < 0 {
		return cfg, nil, errors.New("--clean-padding no puede ser negativo")
	}
	if cfg.markerIndex == 0 {
		return cfg, nil, errors.New("--marker-index no puede ser 0; usa 1 o -1")
	}
	if _, err := parseVisibleMode(cfg.visibleMode); err != nil {
		return cfg, nil, err
	}
	return cfg, fs.Args(), nil
}

func processOne(ctx context.Context, cfg appConfig, anchor pdfmark.Anchor, input string, allowCertGUI bool) (bool, error) {
	matches, err := pdfmark.Locate(ctx, input, pdfmark.LocateOptions{
		Marker:          cfg.marker,
		PDFToTextPath:   cfg.pdftotext,
		CaseInsensitive: cfg.caseInsensitive,
		Timeout:         45 * time.Second,
	})
	if err != nil {
		return false, err
	}
	if len(matches) == 0 {
		return false, fmt.Errorf("no se encontró el marcador %q", cfg.marker)
	}

	if cfg.listMarkers {
		fmt.Printf("%s\n", input)
		for i, m := range matches {
			r := pdfmark.RectForMatch(m, cfg.width, cfg.height, cfg.offsetX, cfg.offsetY, cfg.pageMargin, anchor)
			clean := pdfclean.RectForMarker(m, cfg.cleanPadding)
			fmt.Printf("  #%d página=%d marcador[xMin=%.2f yMin=%.2f xMax=%.2f yMax=%.2f] limpieza[LLX=%.2f LLY=%.2f URX=%.2f URY=%.2f] firma[LLX=%.2f LLY=%.2f URX=%.2f URY=%.2f]\n",
				i+1, m.PageNumber, m.BoxTopLeft.XMin, m.BoxTopLeft.YMin, m.BoxTopLeft.XMax, m.BoxTopLeft.YMax, clean.LLX, clean.LLY, clean.URX, clean.URY, r.LLX, r.LLY, r.URX, r.URY)
		}
		return false, nil
	}

	selected, err := selectMatch(matches, cfg.markerIndex)
	if err != nil {
		return false, err
	}
	rect := pdfmark.RectForMatch(selected, cfg.width, cfg.height, cfg.offsetX, cfg.offsetY, cfg.pageMargin, anchor)
	cleanRect := pdfclean.RectForMarker(selected, cfg.cleanPadding)
	out := outputPath(cfg.outDir, input, cfg.suffix)
	if !cfg.overwrite {
		if _, err := os.Stat(out); err == nil {
			return false, fmt.Errorf("la salida ya existe: %s; usa --overwrite", out)
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}

	if cfg.verbose || cfg.dryRun {
		fmt.Printf("%s -> %s\n", input, out)
		fmt.Printf("  marcador elegido: página=%d; firma LLX=%.2f LLY=%.2f URX=%.2f URY=%.2f\n", rect.Page, rect.LLX, rect.LLY, rect.URX, rect.URY)
		if !cfg.keepMarker {
			fmt.Printf("  limpieza marcador: LLX=%.2f LLY=%.2f URX=%.2f URY=%.2f\n", cleanRect.LLX, cleanRect.LLY, cleanRect.URX, cleanRect.URY)
		}
		if len(matches) > 1 {
			fmt.Printf("  aviso: hay %d marcadores; se usa marker-index=%d\n", len(matches), cfg.markerIndex)
		}
	}

	extra, err := parseExtra(cfg.extra)
	if err != nil {
		return false, err
	}
	mode, err := parseVisibleMode(cfg.visibleMode)
	if err != nil {
		return false, err
	}

	signInput := input
	var tempDir string
	if !cfg.keepMarker {
		if cfg.dryRun {
			cleanOut := filepath.Join(os.TempDir(), "pdfsignmark-clean-preview.pdf")
			cleanRes, err := pdfclean.WhiteoutMarker(ctx, pdfclean.Options{
				GhostscriptPath: cfg.ghostscript,
				InputPath:       input,
				OutputPath:      cleanOut,
				Rect:            cleanRect,
				DryRun:          true,
			})
			if err != nil {
				return false, err
			}
			fmt.Printf("  Ghostscript limpieza: %s\n", pdfclean.ShellQuote(append([]string{cleanRes.Command}, cleanRes.Args...)))
			signInput = cleanOut
		} else {
			tempDir, err = os.MkdirTemp("", "pdfsignmark-*")
			if err != nil {
				return false, fmt.Errorf("no se pudo crear directorio temporal: %w", err)
			}
			defer os.RemoveAll(tempDir)
			signInput = filepath.Join(tempDir, "sin-marcador.pdf")
			cleanRes, err := pdfclean.WhiteoutMarker(ctx, pdfclean.Options{
				GhostscriptPath: cfg.ghostscript,
				InputPath:       input,
				OutputPath:      signInput,
				Rect:            cleanRect,
				Timeout:         2 * time.Minute,
			})
			if cfg.verbose {
				fmt.Printf("  Ghostscript limpieza: %s\n", pdfclean.ShellQuote(append([]string{cleanRes.Command}, cleanRes.Args...)))
				if strings.TrimSpace(cleanRes.Stderr) != "" {
					fmt.Printf("  Ghostscript stderr: %s\n", strings.TrimSpace(cleanRes.Stderr))
				}
			}
			if err != nil {
				return false, err
			}
		}
	}

	effectiveCertGUI := allowCertGUI && shouldUseCertGUI(cfg)
	res, err := autofirma.Sign(ctx, autofirma.SignOptions{
		AutoFirmaPath: cfg.autofirma,
		InputPath:     signInput,
		OutputPath:    out,
		Rect: autofirma.Rect{
			Page: rect.Page,
			LLX:  rect.LLX,
			LLY:  rect.LLY,
			URX:  rect.URX,
			URY:  rect.URY,
		},
		Layer2Text:           cfg.layer2Text,
		FontSize:             cfg.fontSize,
		FontFamily:           cfg.fontFamily,
		FontStyle:            cfg.fontStyle,
		FontColor:            cfg.fontColor,
		Headless:             cfg.headless,
		CertGUI:              effectiveCertGUI,
		GUI:                  cfg.gui,
		Store:                effectiveStore(cfg),
		Password:             cfg.password,
		Alias:                cfg.alias,
		Filter:               cfg.filter,
		Algorithm:            cfg.algorithm,
		Format:               "PAdES",
		ExtraParams:          extra,
		Timeout:              cfg.timeout,
		DryRun:               cfg.dryRun,
		Verbose:              cfg.verbose,
		PreserveConfigSpaces: cfg.preserveConfigSpaces,
		ConfigSeparator:      cfg.configSeparator,
		Visible:              mode == visibleAutoFirma,
	})
	if cfg.dryRun || cfg.verbose {
		fmt.Printf("  AutoFirma: %s\n", autofirma.ShellQuoteRedacted(append([]string{res.Command}, res.Args...)))
		if cfg.verbose && strings.TrimSpace(res.Config) != "" {
			fmt.Println("  Config AutoFirma:")
			for _, line := range strings.Split(autofirma.PrettyConfig(res.Config), "\n") {
				fmt.Printf("    %s\n", line)
			}
		}
	}
	if err != nil {
		return effectiveCertGUI, err
	}
	if res.Stdout != "" && cfg.verbose {
		fmt.Printf("  stdout: %s\n", strings.TrimSpace(res.Stdout))
	}
	if res.Stderr != "" && cfg.verbose {
		fmt.Printf("  stderr: %s\n", strings.TrimSpace(res.Stderr))
	}
	if !cfg.dryRun {
		if mode == visibleAutoFirma {
			vis := autofirma.CheckVisibleSignature(out)
			if cfg.verbose {
				fmt.Printf("  comprobación visible AutoFirma: %s\n", vis.String())
			}
			if cfg.strictVisible && !vis.LikelyVisible {
				return effectiveCertGUI, fmt.Errorf("AutoFirma generó el PDF, pero no se detectó apariencia visible de firma creada por AutoFirma: %s", vis.String())
			}
		}
		fmt.Printf("OK %s\n", out)
	}
	return effectiveCertGUI, nil
}

type visibleMode int

const (
	visibleAutoFirma visibleMode = iota
	visibleNone
)

func parseVisibleMode(s string) (visibleMode, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "_", "-")
	switch s {
	case "", "autofirma", "afirma", "af":
		return visibleAutoFirma, nil
	case "none", "invisible", "sin-visible":
		return visibleNone, nil
	default:
		return visibleAutoFirma, fmt.Errorf("--visible-mode inválido %q; valores válidos: autofirma, none", s)
	}
}

func shouldUseCertGUI(cfg appConfig) bool {
	if cfg.gui || cfg.headless || cfg.noCertGUI {
		return false
	}
	if cfg.certgui {
		return true
	}
	return cfg.password == "" && cfg.alias == "" && cfg.filter == ""
}

func effectiveStore(cfg appConfig) string {
	if strings.TrimSpace(cfg.store) != "" {
		return strings.TrimSpace(cfg.store)
	}
	if runtime.GOOS == "linux" && cfg.password == "" && cfg.alias == "" && cfg.filter == "" {
		return "mozilla"
	}
	return ""
}

func prepareBatchCertificate(ctx context.Context, cfg appConfig, fileCount int) (appConfig, error) {
	if fileCount < 2 || cfg.dryRun || !shouldUseCertGUI(cfg) {
		return cfg, nil
	}

	store := effectiveStore(cfg)
	aliases, err := listCertificateAliases(ctx, cfg, store)
	if err != nil {
		return cfg, fmt.Errorf("no se pudo seleccionar un certificado para el lote: %w", err)
	}
	if len(aliases) == 0 {
		return cfg, fmt.Errorf("AutoFirma no devolvió alias de certificados para --store %s; usa --alias o --filter", store)
	}

	var alias string
	if len(aliases) == 1 {
		alias = aliases[0]
		fmt.Fprintf(os.Stderr, "Usando el único certificado disponible para este lote: %s\n", alias)
	} else {
		if !stdinIsTerminal() {
			return cfg, errors.New("hay varios certificados y no hay terminal interactivo; usa --alias o --filter")
		}
		chosen, err := promptAlias(aliases)
		if err != nil {
			return cfg, err
		}
		alias = chosen
	}

	cfg.alias = alias
	if strings.TrimSpace(cfg.store) == "" {
		cfg.store = store
	}
	cfg.certgui = false
	return cfg, nil
}

func listCertificateAliases(ctx context.Context, cfg appConfig, store string) ([]string, error) {
	res, err := autofirma.ListAliases(ctx, autofirma.AliasOptions{
		AutoFirmaPath: cfg.autofirma,
		Store:         store,
		Password:      cfg.password,
		XML:           true,
		Timeout:       cfg.timeout,
	})
	if err != nil {
		return nil, err
	}
	aliases := parseAliasList(res.Stdout)
	if len(aliases) == 0 {
		aliases = parseAliasList(res.Stderr)
	}
	return aliases, nil
}

func parseAliasList(out string) []string {
	var aliases []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		aliases = append(aliases, s)
	}

	decoder := xml.NewDecoder(strings.NewReader(out))
	var inAlias bool
	var b strings.Builder
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			for _, attr := range t.Attr {
				if strings.EqualFold(attr.Name.Local, "alias") {
					add(attr.Value)
				}
			}
			if strings.EqualFold(t.Name.Local, "alias") {
				inAlias = true
				b.Reset()
			}
		case xml.CharData:
			if inAlias {
				b.Write([]byte(t))
			}
		case xml.EndElement:
			if inAlias && strings.EqualFold(t.Name.Local, "alias") {
				add(b.String())
				inAlias = false
				b.Reset()
			}
		}
	}
	if len(aliases) > 0 {
		return aliases
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "<") || strings.HasPrefix(line, "==") {
			continue
		}
		add(line)
	}
	return aliases
}

func promptAlias(aliases []string) (string, error) {
	fmt.Fprintln(os.Stderr, "Selecciona el certificado que se usará para todo el lote:")
	for i, alias := range aliases {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, alias)
	}
	fmt.Fprintf(os.Stderr, "Número de certificado [1-%d]: ", len(aliases))
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return "", fmt.Errorf("no se pudo leer la selección de certificado: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(aliases) {
		return "", fmt.Errorf("selección de certificado inválida: %q", strings.TrimSpace(line))
	}
	return aliases[n-1], nil
}

func stdinIsTerminal() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func runAliasMode(ctx context.Context, cfg appConfig) error {
	stores := []string{effectiveStore(cfg)}
	if cfg.probeStores {
		stores = []string{"mozilla", "auto"}
		if strings.TrimSpace(cfg.store) != "" {
			stores = append([]string{strings.TrimSpace(cfg.store)}, stores...)
		}
	}
	seen := map[string]bool{}
	for _, store := range stores {
		store = strings.TrimSpace(store)
		if store == "" {
			store = "auto"
		}
		if seen[store] {
			continue
		}
		seen[store] = true
		fmt.Printf("== AutoFirma listaliases --store %s ==\n", store)
		res, err := autofirma.ListAliases(ctx, autofirma.AliasOptions{
			AutoFirmaPath: cfg.autofirma,
			Store:         store,
			Password:      cfg.password,
			XML:           cfg.aliasXML,
			Timeout:       cfg.timeout,
		})
		fmt.Printf("Comando: %s\n", autofirma.ShellQuoteRedacted(append([]string{res.Command}, res.Args...)))
		if strings.TrimSpace(res.Stdout) != "" {
			fmt.Printf("stdout:\n%s\n", strings.TrimSpace(res.Stdout))
		}
		if strings.TrimSpace(res.Stderr) != "" {
			fmt.Printf("stderr:\n%s\n", strings.TrimSpace(res.Stderr))
		}
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			if !cfg.probeStores {
				return err
			}
		} else if strings.TrimSpace(res.Stdout) == "" {
			fmt.Println("Sin salida. El almacén pudo estar vacío o AutoFirma no devolvió alias en stdout.")
		}
		if cfg.probeStores {
			fmt.Println()
		}
	}
	return nil
}

func selectMatch(matches []pdfmark.Match, index int) (pdfmark.Match, error) {
	if len(matches) == 0 {
		return pdfmark.Match{}, errors.New("no hay marcadores")
	}
	if index == -1 {
		return matches[len(matches)-1], nil
	}
	if index < 1 || index > len(matches) {
		return pdfmark.Match{}, fmt.Errorf("--marker-index=%d fuera de rango; hay %d marcador(es)", index, len(matches))
	}
	return matches[index-1], nil
}

func parseExtra(values []string) (map[string]string, error) {
	m := map[string]string{}
	for _, v := range values {
		k, val, ok := strings.Cut(v, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("--extra debe tener forma key=value; recibido %q", v)
		}
		m[strings.TrimSpace(k)] = val
	}
	return m, nil
}

func expandInputs(args []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, arg := range args {
		var candidates []string
		if hasGlob(arg) {
			matches, err := filepath.Glob(arg)
			if err != nil {
				return nil, fmt.Errorf("glob inválido %q: %w", arg, err)
			}
			candidates = matches
		} else {
			candidates = []string{arg}
		}
		for _, c := range candidates {
			st, err := os.Stat(c)
			if err != nil {
				return nil, fmt.Errorf("no se puede acceder a %q: %w", c, err)
			}
			if st.IsDir() {
				continue
			}
			if !strings.EqualFold(filepath.Ext(c), ".pdf") {
				continue
			}
			absOrClean := filepath.Clean(c)
			if !seen[absOrClean] {
				seen[absOrClean] = true
				files = append(files, absOrClean)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func hasGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

func outputPath(outDir, input, suffix string) string {
	base := filepath.Base(input)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return filepath.Join(outDir, name+suffix+ext)
}

func usageError(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	fmt.Fprintln(os.Stderr, "Uso rápido: pdfsignmark [opciones] ./*.pdf")
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
	os.Exit(1)
}
