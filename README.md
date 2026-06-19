# pdfsignmark

CLI en Go para firmar muchos PDFs de golpe con AutoFirma, colocando una firma PAdES visible a partir de una referencia textual dentro de cada documento.

Por defecto busca el texto `[FIRMA]`, pero no está limitado a ese marcador. Puedes usar cualquier palabra o frase del PDF con `--marker`, colocar el cajetín encima, debajo o centrado respecto a esa referencia, y decidir si el texto usado como referencia se oculta visualmente o se conserva.

`pdfsignmark` no implementa criptografía, no accede a claves privadas y no pretende sustituir a AutoFirma ni exponer toda su línea de comandos. La firma real la hace AutoFirma. Esta herramienta solo prepara el PDF cuando hace falta, calcula la posición de la firma visible y llama a AutoFirma con los parámetros necesarios para este flujo de trabajo.

## Funcionalidades

* Firma uno o muchos PDFs en una sola ejecución.
* Busca una referencia textual en cada PDF; por defecto `[FIRMA]`.
* Permite usar cualquier frase del documento como referencia con `--marker`.
* Coloca la firma centrada, encima o debajo de esa referencia con `--anchor` y offsets.
* Puede ocultar visualmente el marcador o conservarlo con `--keep-marker`.
* Reutiliza el certificado elegido al firmar lotes, evitando seleccionar certificado para cada PDF.
* Permite listar marcadores, probar almacenes de certificados y hacer `--dry-run` antes de firmar.
* Puede generar una firma sin cajetín visible con `--visible-mode none`, manteniendo el flujo de localización por marcador.

## Cómo funciona

1. Extrae coordenadas de texto del PDF con `pdftotext -bbox`.
2. Localiza el marcador elegido, por defecto `[FIRMA]`.
3. Calcula el rectángulo de la firma visible usando `--anchor`, `--offset-x`, `--offset-y`, `--width` y `--height`.
4. Opcionalmente oculta visualmente el texto localizado con Ghostscript antes de firmar.
5. Invoca AutoFirma para generar la firma PAdES.

Esto permite trabajar de dos formas habituales:

* Plantillas con un marcador técnico como `[FIRMA]`, que normalmente se oculta.
* Documentos con una frase real como `Firmado por solicitante`, donde puedes poner la firma encima o debajo sin ocultar la frase.

## Dependencias

En Debian/Ubuntu:

```bash
sudo apt update
sudo apt install -y poppler-utils ghostscript
```

También necesitas AutoFirma instalado. En Linux suele estar disponible como `/usr/bin/autofirma`, `/usr/bin/AutoFirma` o en el `PATH`.

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

Ejecutar tests:

```bash
make test
```

Crear un paquete Debian local:

```bash
make deb
```

## Uso básico

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

Si necesitas que la búsqueda no distinga mayúsculas y minúsculas:

```bash
./bin/pdfsignmark \
  --marker 'firmado por solicitante' \
  --case-insensitive \
  --list-markers \
  documento.pdf
```

## Colocar la firma

El cajetín visible mide por defecto `220 x 80` puntos PDF. Puedes cambiarlo con:

```bash
./bin/pdfsignmark --width 220 --height 80 --overwrite --v documento.pdf
```

El anclaje controla cómo se calcula la firma desde el texto encontrado:

* `--anchor center`: centra la firma sobre el marcador. Es el valor por defecto.
* `--anchor lower-left`: coloca la esquina inferior izquierda de la firma en la esquina inferior izquierda del marcador. En la práctica, la firma crece hacia arriba desde la referencia.
* `--anchor top-left`: coloca la esquina superior izquierda de la firma en la esquina superior izquierda del marcador. En la práctica, la firma queda hacia abajo desde la referencia.

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

Para evitar que el cajetín quede pegado al borde de página, `pdfsignmark` aplica un margen mínimo de 5 puntos PDF. Puedes cambiarlo con:

```bash
./bin/pdfsignmark --page-margin 10 --overwrite --v documento.pdf
```

Si hay varios marcadores:

```bash
# Usar el último.
./bin/pdfsignmark --marker-index -1 --overwrite --v documento.pdf
```

```bash
# Usar el segundo.
./bin/pdfsignmark --marker-index 2 --overwrite --v documento.pdf
```

`--marker-index 0` no es válido. Usa `1` para el primero, `2` para el segundo o `-1` para el último.

## Ocultar o conservar la referencia

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

Si Ghostscript no está en el `PATH`, puedes indicar su ruta:

```bash
./bin/pdfsignmark --gs /ruta/a/gs --overwrite --v documento.pdf
```

## Certificados

En Linux, en el caso interactivo habitual, si no indicas `--store`, `--alias`, `--filter` ni `--password`, la aplicación usa por defecto el almacén Mozilla y selección gráfica de certificado:

```text
-certgui -store mozilla
```

Esto suele funcionar bien cuando los certificados están en Firefox/Mozilla.

Opciones como `--no-certgui`, `--gui`, `--headless`, `--alias`, `--filter`, `--password` o un `--store` explícito modifican ese flujo.

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

Usar un filtro de certificado:

```bash
./bin/pdfsignmark \
  --store mozilla \
  --filter 'subject.contains:12345678Z;nonexpired:' \
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

Evitar el selector gráfico:

```bash
./bin/pdfsignmark \
  --store mozilla \
  --alias 'ALIAS_ELEGIDO' \
  --no-certgui \
  --overwrite \
  --v \
  documento.pdf
```

## Diagnóstico

Ver qué haría sin firmar:

```bash
./bin/pdfsignmark --dry-run --v documento.pdf
```

La salida muestra el marcador elegido, el rectángulo de limpieza si aplica, el rectángulo de firma y el comando de AutoFirma.

Listar marcadores con otro binario `pdftotext`:

```bash
./bin/pdfsignmark \
  --pdftotext /ruta/a/pdftotext \
  --list-markers \
  documento.pdf
```

Usar un binario concreto de AutoFirma:

```bash
./bin/pdfsignmark \
  --autofirma /ruta/a/autofirma \
  --overwrite \
  --v \
  documento.pdf
```

Continuar con el resto de PDFs si uno falla:

```bash
./bin/pdfsignmark \
  --continue-on-error \
  --overwrite \
  --v \
  ./*.pdf
```

Cambiar el tiempo máximo por PDF para AutoFirma:

```bash
./bin/pdfsignmark \
  --timeout 5m \
  --overwrite \
  --v \
  documento.pdf
```

Comprobar heurísticamente que AutoFirma ha creado una apariencia visible:

```bash
./bin/pdfsignmark \
  --strict-visible \
  --overwrite \
  --v \
  documento.pdf
```

Esta comprobación no valida criptográficamente la firma. Solo intenta detectar si el PDF firmado contiene una apariencia visible de firma.

## Firma sin cajetín visible

Para generar la firma sin cajetín visual de AutoFirma:

```bash
./bin/pdfsignmark --visible-mode none --overwrite --v documento.pdf
```

`--visible-mode none` desactiva la apariencia visible de AutoFirma, pero `pdfsignmark` sigue necesitando encontrar el marcador. El marcador se usa para mantener el mismo flujo de procesamiento y, salvo que uses `--keep-marker`, se ocultará visualmente antes de firmar.

Valores admitidos:

* `autofirma`: modo visible normal.
* `none`: sin cajetín visible.

También se aceptan algunos alias de compatibilidad como `afirma`, `af`, `invisible` y `sin-visible`.

## Texto personalizado dentro del cajetín

AutoFirma puede mostrar su texto por defecto dentro del cajetín visible. Ese es el modo más compatible, especialmente en Linux.

Si quieres personalizar el texto:

```bash
./bin/pdfsignmark \
  --text 'Firmado_por_$$SUBJECTCN$$' \
  --overwrite \
  --v \
  documento.pdf
```

Si al usar `--text` deja de verse la firma, quítalo y deja que AutoFirma use su texto por defecto.

Cuando `--text` está presente, también puedes ajustar parámetros de fuente:

```bash
./bin/pdfsignmark \
  --text 'Firmado_por_$$SUBJECTCN$$' \
  --font-size 8 \
  --font-family 1 \
  --font-style 0 \
  --font-color black \
  --overwrite \
  --v \
  documento.pdf
```

Valores de `--font-family`:

* `0`: Courier
* `1`: Helvetica
* `2`: Times
* `3`: Symbol
* `4`: ZapfDingBats

Valores habituales de `--font-style`:

* `0`: normal
* `1`: negrita
* `2`: cursiva
* `3`: negrita + cursiva
* `4`: subrayado
* `8`: tachado

Colores admitidos por la opción de ayuda:

* `black`
* `white`
* `gray`
* `lightGray`
* `darkGray`
* `red`
* `pink`

Por defecto, `pdfsignmark` sustituye espacios del texto de configuración por guiones bajos para evitar problemas con algunos wrappers de AutoFirma en Linux. Si tu AutoFirma respeta bien argumentos con espacios, puedes conservarlos con:

```bash
./bin/pdfsignmark \
  --text 'Firmado por $$SUBJECTCN$$' \
  --preserve-config-spaces \
  --overwrite \
  --v \
  documento.pdf
```

## Salida

Por defecto, la salida se crea en el directorio actual con el sufijo `_firmado`.

Ejemplo:

```text
documento.pdf -> documento_firmado.pdf
```

Cambiar directorio de salida:

```bash
./bin/pdfsignmark \
  --out-dir firmados \
  --overwrite \
  --v \
  documento.pdf
```

Cambiar sufijo:

```bash
./bin/pdfsignmark \
  --suffix _signed \
  --overwrite \
  --v \
  documento.pdf
```

Si la salida ya existe, `pdfsignmark` falla salvo que uses `--overwrite`.

## Opciones avanzadas

El README cubre el flujo habitual. Para ver todas las opciones soportadas por esta herramienta:

```bash
./bin/pdfsignmark --help
```

Algunas opciones existen principalmente para diagnóstico, compatibilidad o entornos concretos:

* `--pdftotext`: ruta del binario `pdftotext`.
* `--autofirma`: ruta del binario o script de AutoFirma.
* `--gs`: ruta del binario Ghostscript.
* `--timeout`: tiempo máximo por PDF para AutoFirma.
* `--config-separator`: separador interno usado en `-config`; valores habituales: `literal` o `newline`.
* `--preserve-config-spaces`: conserva espacios en valores de configuración.
* `--extra key=value`: añade un parámetro adicional al `-config` de AutoFirma. Puede repetirse.
* `--strict-visible`: falla si no se detecta heurísticamente una apariencia visible en el PDF firmado.
* `--algorithm`: algoritmo de firma; por defecto `SHA256withRSA`.
* `--headless`: configura `headLess=true` en AutoFirma.
* `--gui`: ejecuta AutoFirma con entorno gráfico.
* `--no-certgui`: evita el selector gráfico de certificado.

`pdfsignmark` solo usa los parámetros de AutoFirma necesarios para su caso de uso. Si necesitas operaciones generales de AutoFirma fuera de este flujo, usa AutoFirma directamente.

## Limitaciones

* El marcador debe existir como texto real del PDF. Si el PDF es una imagen escaneada, `pdftotext` no lo encontrará.
* La limpieza del marcador es visual, no semántica: el texto puede seguir existiendo internamente.
* Si el fondo del PDF no es blanco, la limpieza visual puede dejar un rectángulo visible.
* No edites el PDF después de firmarlo; cualquier modificación posterior puede invalidar la firma.
* Algunas versiones de AutoFirma en Linux solo aceptan coordenadas enteras para la firma visible. `pdfsignmark` las redondea automáticamente.
* La comprobación de firma visible de `--strict-visible` es heurística; no sustituye a una validación criptográfica.
