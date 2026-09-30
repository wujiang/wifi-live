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

clean:
	rm -rf wifi_live WifiInfo.app

.PHONY: all clean
