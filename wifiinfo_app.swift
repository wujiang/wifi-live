// Background helper: writes this Mac's Wi-Fi link (incl. BSSID, which needs
// Location permission) to .wifi_now.json next to WifiInfo.app once a second.
import AppKit
import CoreLocation
import CoreWLAN

// the folder holding WifiInfo.app, which is where wifi_live runs from
let dir = Bundle.main.bundleURL.deletingLastPathComponent().path

class D: NSObject, CLLocationManagerDelegate {
  let m = CLLocationManager()
  let out = URL(fileURLWithPath: dir + "/.wifi_now.json")
  func start() {
    m.delegate = self
    m.requestWhenInUseAuthorization()
    Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { _ in
      self.write()
      // wifi_live touches .wifi_scan_req when saving a spot: scan right away
      let req = dir + "/.wifi_scan_req"
      if let a = try? FileManager.default.attributesOfItem(atPath: req),
         let d = a[.modificationDate] as? Date, d > lastScan {
        DispatchQueue.global().async { scan() }
      }
    }
    // scan for all units every 20 s, off the main thread (a scan takes a few seconds)
    Timer.scheduledTimer(withTimeInterval: 20, repeats: true) { _ in DispatchQueue.global().async { scan() } }
    DispatchQueue.global().async { scan() }
    // quit after 10 min without anyone reading (wifi_live touches .wifi_want)
    Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { _ in
      let want = dir + "/.wifi_want"
      if let a = try? FileManager.default.attributesOfItem(atPath: want),
         let d = a[.modificationDate] as? Date, Date().timeIntervalSince(d) < 600 { return }
      NSApp.terminate(nil)
    }
  }
  func write() {
    let i = CWWiFiClient.shared().interface()
    let o: [String: Any] = [
      "t": Date().timeIntervalSince1970,
      "auth": m.authorizationStatus.rawValue,
      "bssid": i?.bssid() ?? "", "ssid": i?.ssid() ?? "",
      "rssi": i?.rssiValue() ?? 0, "noise": i?.noiseMeasurement() ?? 0,
      "rate": Int(i?.transmitRate() ?? 0), "ch": i?.wlanChannel()?.channelNumber ?? 0]
    if let d = try? JSONSerialization.data(withJSONObject: o) { try? d.write(to: out, options: .atomic) }
  }
  func locationManagerDidChangeAuthorization(_ m: CLLocationManager) { write() }
}
// `open -g WifiInfo.app --args scan`: write every visible access point to
// .wifi_scan.json and quit, instead of running the 1-second loop.
var scanning = false
var lastScan = Date.distantPast
let scanLock = NSLock()

func scan() {
  scanLock.lock()
  if scanning { scanLock.unlock(); return }
  scanning = true; lastScan = Date()
  scanLock.unlock()
  defer { scanLock.lock(); scanning = false; scanLock.unlock() }
  var list: [[String: Any]] = []
  if let i = CWWiFiClient.shared().interface(), let nets = try? i.scanForNetworks(withSSID: nil) {
    for n in nets {
      list.append(["ssid": n.ssid ?? "", "bssid": n.bssid ?? "", "rssi": n.rssiValue,
                   "ch": n.wlanChannel?.channelNumber ?? 0,
                   "band": n.wlanChannel?.channelBand == .band2GHz ? "2.4" : (n.wlanChannel?.channelBand == .band6GHz ? "6" : "5")])
    }
  }
  let o: [String: Any] = ["t": Date().timeIntervalSince1970, "auth": CLLocationManager().authorizationStatus.rawValue, "nets": list]
  if let data = try? JSONSerialization.data(withJSONObject: o) {
    try? data.write(to: URL(fileURLWithPath: dir + "/.wifi_scan.json"), options: .atomic)
  }
}

let app = NSApplication.shared
app.setActivationPolicy(.accessory)
if CommandLine.arguments.contains("scan") {
  scan()
  exit(0)
}
let d = D(); d.start()
app.run()
