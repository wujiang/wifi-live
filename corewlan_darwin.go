package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework CoreWLAN -framework Foundation
#import <CoreWLAN/CoreWLAN.h>

typedef struct { int ok, rssi, noise, rate, ch; } wlan_link;

// Reading signal, noise, rate and channel needs no Location permission, unlike the BSSID.
static wlan_link read_link(void) {
	wlan_link l = {0};
	@autoreleasepool {
		CWInterface *i = [[CWWiFiClient sharedWiFiClient] interface];
		if (i) {
			l.ok = 1;
			l.rssi = (int)[i rssiValue];
			l.noise = (int)[i noiseMeasurement];
			l.rate = (int)[i transmitRate];
			l.ch = (int)[[i wlanChannel] channelNumber];
		}
	}
	return l;
}
*/
import "C"

// coreWLANLink reads this Mac's Wi-Fi link straight from CoreWLAN; nil when not connected.
func coreWLANLink() *Link {
	l := C.read_link()
	if l.ok == 0 || l.rssi == 0 {
		return nil
	}
	return &Link{RSSI: int(l.rssi), Noise: int(l.noise), Rate: int(l.rate), Ch: int(l.ch)}
}
