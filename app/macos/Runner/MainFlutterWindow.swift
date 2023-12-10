import Cocoa
import FlutterMacOS
import NetworkExtension

class MainFlutterWindow: NSWindow {
  var providerManager: NETunnelProviderManager!

  override func awakeFromNib() {
    let flutterViewController = FlutterViewController()
    let windowFrame = frame
    contentViewController = flutterViewController
    setFrame(windowFrame, display: true)

    RegisterGeneratedPlugins(registry: flutterViewController)
    let batteryChannel = FlutterMethodChannel(name: "bsrouter", binaryMessenger: flutterViewController.engine.binaryMessenger)
    batteryChannel.setMethodCallHandler {
      (call: FlutterMethodCall, result: @escaping FlutterResult) in
      switch call.method {
      case "vpn.load":
        self.loadVPN(result: result)
      case "vpn.config":
        let args = call.arguments as! [String]
        self.configVPN(name: args[0], channel: args[1], mode: args[2], config: args[3], gfwRules: args[4], userRules: args[5], result: result)
      case "vpn.start":
        self.startVPN(result: result)
      case "vpn.stop":
        self.stopVPN(result: result)
      default:
        result(FlutterError(code: "METHOD_NOT_FOUND", message: "method not found", details: nil))
      }
    }

    super.awakeFromNib()
  }

  func loadVPN(result: @escaping FlutterResult) {
    NETunnelProviderManager.loadAllFromPreferences { managers, error in
      if error != nil {
        result(FlutterError(code: "VPN_ERROR", message: "list vpn config", details: error))
        return
      }
      var message = [String: Any]()
      if let managers = managers {
        if let manager = managers.first {
          self.providerManager = manager
        } else {
          message["state"] = "NONE"
          result(message)
          return
        }
      } else {
        message["state"] = "NONE"
        result(message)
        return
      }
      self.providerManager.loadFromPreferences(completionHandler: { error in
        if error != nil {
          result(FlutterError(code: "VPN_ERROR", message: "load vpn config", details: error))
          return
        }
        let config = self.providerManager.protocolConfiguration as! NETunnelProviderProtocol
        switch self.providerManager.connection.status {
        case .invalid:
          message["state"] = "ERROR"
        case .disconnected:
          message["state"] = "STOPPED"
        case .connecting:
          message["state"] = "STARTING"
        case .connected:
          message["state"] = "RUNNING"
        case .reasserting:
          message["state"] = "STOPPING"
        case .disconnecting:
          message["state"] = "STOPPING"
          break
        @unknown default:
          message["state"] = "ERROR"
        }
        message["name"] = config.providerConfiguration?["name"]
        message["config"] = config.providerConfiguration?["config"]
        message["channel"] = config.providerConfiguration?["channel"]
        result(message)
      })
    }
  }

  func configVPN(name: String, channel: String, mode: String, config: String, gfwRules: String, userRules: String, result: @escaping FlutterResult) {
    NETunnelProviderManager.loadAllFromPreferences { managers, error in
      if error != nil {
        result(FlutterError(code: "VPN_ERROR", message: "list vpn config", details: error))
        return
      }
      self.providerManager = managers?.first ?? NETunnelProviderManager()
      self.providerManager?.loadFromPreferences { error in
        if error != nil {
          result(FlutterError(code: "VPN_ERROR", message: "load vpn config", details: error))
          return
        }
        let tunnelProtocol = NETunnelProviderProtocol()
        tunnelProtocol.username = "user"
        tunnelProtocol.serverAddress = "127.0.0.1"
        tunnelProtocol.providerBundleIdentifier = "com.example.app.Proxy" // bundle id of the network extension target
        tunnelProtocol.providerConfiguration = ["name": name, "channel": channel, "mode": mode, "config": config, "gfwRules": gfwRules, "userRules": userRules]
        tunnelProtocol.disconnectOnSleep = false
        self.providerManager.protocolConfiguration = tunnelProtocol
        self.providerManager.localizedDescription = name
        self.providerManager.isEnabled = true
        self.providerManager.saveToPreferences(completionHandler: { error in
          if error != nil {
              result(FlutterError(code: "VPN_ERROR", message: "save vpn config", details: error.debugDescription))
            return
          }
          result("OK")
        })
      }
    }
  }

  func startVPN(result: @escaping FlutterResult) {
    NETunnelProviderManager.loadAllFromPreferences { managers, error in
      if error != nil {
        result(FlutterError(code: "VPN_ERROR", message: "list vpn config", details: error))
        return
      }
      if let managers = managers {
        if let manager = managers.first {
          self.providerManager = manager
        } else {
          result(FlutterError(code: "VPN_ERROR", message: "not vpn config", details: error))
          return
        }
      } else {
        result(FlutterError(code: "VPN_ERROR", message: "not vpn config", details: error))
        return
      }
      self.providerManager.loadFromPreferences(completionHandler: { error in
        if error != nil {
          result(FlutterError(code: "VPN_ERROR", message: "load vpn config", details: error))
          return
        }
        do {
          try self.providerManager.connection.startVPNTunnel() // starts the VPN tunnel.
          result("OK")
        } catch {
          result(FlutterError(code: "VPN_ERROR", message: "start vpn error", details: error))
        }
      })
    }
  }

  func stopVPN(result: @escaping FlutterResult) {
    NETunnelProviderManager.loadAllFromPreferences { managers, error in
      if error != nil {
        result(FlutterError(code: "VPN_ERROR", message: "list vpn config", details: error))
        return
      }
      if let managers = managers {
        if let manager = managers.first {
          self.providerManager = manager
        } else {
          result(FlutterError(code: "VPN_ERROR", message: "not vpn config", details: error))
          return
        }
      } else {
        result(FlutterError(code: "VPN_ERROR", message: "not vpn config", details: error))
        return
      }
      self.providerManager.loadFromPreferences(completionHandler: { error in
        if error != nil {
          result(FlutterError(code: "VPN_ERROR", message: "load vpn config", details: error))
          return
        }
        self.providerManager.connection.stopVPNTunnel() // starts the VPN tunnel.
        result("OK")
      })
    }
  }
}
