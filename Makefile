# Builds everything into this folder: wifi_live and WifiInfo.app must sit together.
all: wifi_live WifiInfo.app

wifi_live: $(wildcard *.go) go.mod go.sum
	go build -o $@ .

WifiInfo.app: wifiinfo_app.swift Info.plist
	mkdir -p $@/Contents/MacOS
	cp Info.plist $@/Contents/Info.plist
	swiftc -O -o $@/Contents/MacOS/WifiInfo wifiinfo_app.swift
	codesign --force -s - $@
	touch $@

# Release zip: universal (Apple silicon + Intel) builds of both, for macOS 12+.
VERSION ?= $(shell git describe --tags --always --dirty)
DIST = dist/wifi-live-$(VERSION)-macos
MIN_MACOS = 12.0

dist:
	rm -rf $(DIST) $(DIST).zip && mkdir -p $(DIST)/WifiInfo.app/Contents/MacOS
	for a in arm64 amd64; do \
		CGO_ENABLED=1 GOARCH=$$a MACOSX_DEPLOYMENT_TARGET=$(MIN_MACOS) \
			go build -trimpath -ldflags=-s -o dist/wifi_live.$$a . || exit 1; \
	done
	lipo -create -output $(DIST)/wifi_live dist/wifi_live.arm64 dist/wifi_live.amd64
	for a in arm64 x86_64; do \
		swiftc -O -target $$a-apple-macos$(MIN_MACOS) -o dist/WifiInfo.$$a wifiinfo_app.swift || exit 1; \
	done
	lipo -create -output $(DIST)/WifiInfo.app/Contents/MacOS/WifiInfo dist/WifiInfo.arm64 dist/WifiInfo.x86_64
	cp Info.plist $(DIST)/WifiInfo.app/Contents/Info.plist
	codesign --force -s - $(DIST)/WifiInfo.app
	codesign --force -s - $(DIST)/wifi_live
	cp README.md wifi_live.example.json $(DIST)/
	rm -f dist/wifi_live.* dist/WifiInfo.*
	cd dist && ditto -c -k --keepParent $(notdir $(DIST)) $(notdir $(DIST)).zip

clean:
	rm -rf wifi_live WifiInfo.app dist

.PHONY: all dist clean
