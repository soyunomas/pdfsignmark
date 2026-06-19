PACKAGE := pdfsignmark
VERSION ?= 0.1.0
ARCH ?= $(shell dpkg --print-architecture 2>/dev/null || echo amd64)
DIST_DIR := dist
DEB_ROOT := $(DIST_DIR)/$(PACKAGE)_$(VERSION)_$(ARCH)
DEB := $(DIST_DIR)/$(PACKAGE)_$(VERSION)_$(ARCH).deb
HOMEPAGE := https://github.com/Soyunomas/$(PACKAGE)
DESCRIPTION := CLI en Go para firmar PDFs con AutoFirma colocando la firma visible donde aparezca el marcador [FIRMA].

.PHONY: build test clean clean-obsolete deb

# Removes directories left by older pdfsignmark zips. `unzip -o` overwrites
# files but does not delete files that disappeared from newer releases.
clean-obsolete:
	rm -rf internal/stamp

test: clean-obsolete
	go test ./...

build: test
	mkdir -p bin
	go build -buildvcs=false -trimpath -ldflags='-s -w' -o bin/pdfsignmark ./cmd/pdfsignmark

clean: clean-obsolete
	rm -rf bin dist

deb: build
	rm -rf "$(DEB_ROOT)"
	mkdir -p "$(DEB_ROOT)/DEBIAN" "$(DEB_ROOT)/usr/bin" "$(DEB_ROOT)/usr/share/doc/$(PACKAGE)"
	install -m 0755 bin/pdfsignmark "$(DEB_ROOT)/usr/bin/pdfsignmark"
	install -m 0644 README.md LICENSE "$(DEB_ROOT)/usr/share/doc/$(PACKAGE)/"
	printf 'Package: $(PACKAGE)\nVersion: $(VERSION)\nSection: utils\nPriority: optional\nArchitecture: $(ARCH)\nDepends: poppler-utils, ghostscript\nRecommends: autofirma\nMaintainer: Soyunomas <Soyunomas@users.noreply.github.com>\nHomepage: $(HOMEPAGE)\nDescription: $(DESCRIPTION)\n pdfsignmark localiza el marcador [FIRMA] en PDF, lo oculta visualmente y delega la firma PAdES visible en AutoFirma.\n' > "$(DEB_ROOT)/DEBIAN/control"
	dpkg-deb --build --root-owner-group "$(DEB_ROOT)" "$(DEB)"
	@printf 'Paquete generado: %s\n' "$(DEB)"
