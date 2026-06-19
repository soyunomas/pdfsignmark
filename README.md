# pdfsignmark

CLI en Go para firmar muchos PDFs de golpe con AutoFirma, colocando una firma PAdES visible a partir de una referencia textual dentro de cada documento.

Por defecto busca el texto `[FIRMA]`, pero no está limitado a ese marcador. Puedes usar cualquier palabra o frase del PDF con `--marker`, colocar el cajetín encima, debajo o centrado respecto a esa referencia, y decidir si el texto usado como referencia se oculta visualmente o se conserva.

La aplicación no implementa criptografía ni accede a claves privadas. La firma real la hace AutoFirma.

## Funcionalidades

- Firma uno o muchos PDFs en una sola ejecución.
- Busca una referencia textual en cada PDF; por defecto `[FIRMA]`.
- Permite usar cualquier frase del documento como referencia con `--marker`.
- Coloca la firma centrada, encima o debajo de esa referencia con `--anchor` y offsets.
- Puede ocultar visualmente el marcador o conservarlo con `--keep-marker`.
- Reutiliza el certificado elegido al firmar lotes, evitando seleccionar certificado para cada PDF.
- Permite listar marcadores, probar almacenes de certificados y hacer `--dry-run` antes de firmar.

## Cómo Funciona

1. Extrae coordenadas de texto del PDF con `pdftotext -bbox`.
2. Localiza el marcador elegido, por defecto `[FIRMA]`.
3. Calcula el rectángulo de la firma visible usando `--anchor`, `--offset-x`, `--offset-y`, `--width` y `--height`.
4. Opcionalmente oculta visualmente el texto localizado con Ghostscript antes de firmar.
5. Invoca AutoFirma para generar la firma PAdES visible.

Esto permite trabajar de dos formas habituales:

- Plantillas con un marcador técnico como `[FIRMA]`, que normalmente se oculta.
- Documentos con una frase real como `Firmado por solicitante`, donde puedes poner la firma encima o debajo sin ocultar la frase.

## Dependencias

En Debian/Ubuntu:

```bash
sudo apt update
sudo apt install -y poppler-utils ghostscript
```

También necesitas AutoFirma instalado. En Linux suele estar disponible como `/usr/bin/autofirma` o en el `PATH`.

Ghostscript solo es necesario si quieres ocultar visualmente el marcador. Si usas `--keep-marker`, no se ejecuta la limpieza previa.

## Compilar

```bash
make clean
make build
```

El binario queda en:

```bash
./bin/pdfsignmark
```

Crear un paquete Debian local:

```bash
make deb
```

## Uso Básico

Ver dónde detecta el marcador por defecto (`[FIRMA]`):

```bash
./bin/pdfsignmark --list-markers documento.pdf
```

Firmar un PDF:

```bash
./bin/pdfsignmark --overwrite --v documento.pdf
```

Por defecto genera `documento_firmado.pdf` en el directorio actual.

Firmar todos los PDF de una carpeta:

```bash
./bin/pdfsignmark --overwrite --v ./*.pdf
```

Usar otro texto como referencia:

```bash
./bin/pdfsignmark --marker 'Firmado por solicitante' --list-markers documento.pdf
```

```bash
./bin/pdfsignmark \
  --marker 'Firmado por solicitante' \
  --keep-marker \
  --overwrite \
  --v \
  documento.pdf
```

`pdfsignmark` ignora espacios internos introducidos por `pdftotext` y tolera puntuación pegada al final, así que una búsqueda como `Firmado por solicitante` puede encontrar `Firmado por solicitante:`.

## Colocar La Firma

El cajetín visible mide por defecto `220 x 80` puntos PDF. Puedes cambiarlo con:

```bash
./bin/pdfsignmark --width 220 --height 80 --overwrite --v documento.pdf
```

El anclaje controla cómo se calcula la firma desde el texto encontrado:

- `--anchor center`: centra la firma sobre el marcador. Es el valor por defecto.
- `--anchor lower-left`: coloca la esquina inferior izquierda de la firma en la esquina inferior izquierda del marcador. En la práctica, la firma crece hacia arriba desde la referencia.
- `--anchor top-left`: coloca la esquina superior izquierda de la firma en la esquina superior izquierda del marcador. En la práctica, la firma queda hacia abajo desde la referencia.

Ejemplos:

```bash
# Firma centrada sobre [FIRMA].
./bin/pdfsignmark --overwrite --v documento.pdf
```

```bash
# Firma encima de una frase, conservando la frase visible.
./bin/pdfsignmark \
  --marker 'Firmado por solicitante' \
  --anchor lower-left \
  --offset-y 8 \
  --keep-marker \
  --overwrite \
  --v \
  documento.pdf
```

```bash
# Firma debajo de una frase, conservando la frase visible.
./bin/pdfsignmark \
  --marker 'Firmado por solicitante' \
  --anchor top-left \
  --offset-y -8 \
  --keep-marker \
  --overwrite \
  --v \
  documento.pdf
```

Los desplazamientos están en puntos PDF. `--offset-x` mueve horizontalmente y `--offset-y` mueve verticalmente; valores positivos de `--offset-y` suben la firma.

Si hay varios marcadores:

```bash
# Usar el último.
./bin/pdfsignmark --marker-index -1 --overwrite --v documento.pdf
```

```bash
# Usar el segundo.
./bin/pdfsignmark --marker-index 2 --overwrite --v documento.pdf
```

## Ocultar O Conservar La Referencia

Por defecto, `pdfsignmark` oculta visualmente el texto localizado antes de firmar. Esto es útil si el marcador es algo como `[FIRMA]` y no quieres que aparezca en el PDF final.

```bash
./bin/pdfsignmark --overwrite --v documento.pdf
```

Para conservar el texto de referencia:

```bash
./bin/pdfsignmark --keep-marker --overwrite --v documento.pdf
```

Para ajustar cuánto blanco se pinta alrededor del texto ocultado:

```bash
./bin/pdfsignmark --clean-padding 4 --overwrite --v documento.pdf
```

La ocultación es visual: pinta un rectángulo blanco sobre el texto antes de firmar. No elimina el texto interno del PDF. Si el fondo no es blanco, usa `--keep-marker` o ajusta la plantilla.

## Certificados

En Linux, si no indicas `--store` y tampoco usas `--alias` o `--filter`, la aplicación usa por defecto:

```text
-certgui -store mozilla
```

Esto suele funcionar bien cuando los certificados están en Firefox/Mozilla.

Probar almacenes:

```bash
./bin/pdfsignmark --probe-stores
```

Listar alias:

```bash
./bin/pdfsignmark --list-aliases --store mozilla
```

Usar un alias concreto:

```bash
./bin/pdfsignmark \
  --store mozilla \
  --alias 'ALIAS_ELEGIDO' \
  --overwrite \
  --v \
  ./*.pdf
```

Usar un certificado PKCS#12:

```bash
./bin/pdfsignmark \
  --store 'pkcs12:/home/yo/certificado.p12' \
  --password 'CONTRASEÑA' \
  --overwrite \
  --v \
  documento.pdf
```

Usar DNIe:

```bash
./bin/pdfsignmark --store dni --certgui --overwrite --v documento.pdf
```

## Diagnóstico

Ver qué haría sin firmar:

```bash
./bin/pdfsignmark --dry-run --v documento.pdf
```

La salida muestra el marcador elegido, el rectángulo de limpieza si aplica, el rectángulo de firma y el comando de AutoFirma.

Firma invisible, sin cajetín visual:

```bash
./bin/pdfsignmark --visible-mode none --overwrite --v documento.pdf
```

Texto personalizado dentro del cajetín:

```bash
./bin/pdfsignmark \
  --text 'Firmado_por_$$SUBJECTCN$$' \
  --overwrite \
  --v \
  documento.pdf
```

El modo más compatible con AutoFirma en Linux es no personalizar `--text`. Si al usarlo deja de verse la firma, quítalo.

## Limitaciones

- El marcador debe existir como texto real del PDF. Si el PDF es una imagen escaneada, `pdftotext` no lo encontrará.
- La limpieza del marcador es visual, no semántica: el texto puede seguir existiendo internamente.
- No edites el PDF después de firmarlo; cualquier modificación posterior puede invalidar la firma.
- Algunas versiones de AutoFirma en Linux solo aceptan coordenadas enteras para la firma visible. `pdfsignmark` las redondea automáticamente.
