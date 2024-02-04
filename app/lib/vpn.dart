import 'dart:convert';
import 'package:flutter/services.dart';
import 'package:logging/logging.dart';

const platform = MethodChannel('bsrouter');
final _log = Logger('vpn');

// class VpnConfigBuilder {
//   String key;
//   String region;
//   String server;
//   String dns;
//   Map<String, dynamic> config = {};

//   VpnConfigBuilder(this.key, this.region, this.server, this.dns, String name) {
//     var filter = "";
//     if (region.isEmpty || region == "auto") {
//       filter = "";
//     } else {
//       filter = "$region-";
//     }
//     config["name"] = name;
//     config["forwards"] = {
//       "tun~[[TUN]]": "S-$filter.*->\${HOST}",
//       // "tun~tun://[[GW]]>[[FD]]": "S-$filter.*->\${HOST}",
//       // "tun~gw://127.0.0.1:6238>127.0.0.1:6235": "S-$filter.*->\${HOST}",
//     };
//     Map<String, dynamic> channels = {};
//     channels["keep_max"] = 10;
//     config["channels"] = channels;
//     config["web"] = {
//       "web": "tcp://0.0.0.0:30100",
//     };
//   }

//   VpnConfigBuilder addChannel(
//       String name, String remote, String token, String ca) {
//     var channels = config["channels"] as Map<String, dynamic>;
//     channels[name] = {
//       "remote": remote,
//       "domain": "openworkconnect.loc",
//       "token": token,
//       "tls_ca": ca,
//       "keep": 3,
//     };
//     return this;
//   }

//   VpnConfig build() {
//     var result = VpnConfig();
//     result.key = key;
//     if (region.isEmpty) {
//       result.region = "auto";
//     } else {
//       result.region = region;
//     }
//     result.dns = dns;
//     result.server = server;
//     result.config = jsonEncode(config);
//     return result;
//   }
// }

class VpnConfig {
  String name;
  String channel;
  String mode;
  String config;
  String gfwRules;
  String userRules;
  VpnState state;

  VpnConfig()
      : name = "",
        channel = "",
        mode = "",
        config = "",
        gfwRules = "",
        userRules = "",
        state = VpnState.none;
  VpnConfig.error()
      : name = "",
        channel = "",
        mode = "",
        config = "",
        gfwRules = "",
        userRules = "",
        state = VpnState.error;

  static VpnConfig testLoc() {
    var vc = VpnConfig();
    vc.name = "app";
    vc.channel = ".*";
    vc.mode = "none";
    vc.config = jsonEncode({
      "name": "app",
      "channels": {
        "N0": {
          "remote": "tls://192.168.1.7:10023",
          "token": "123",
          "tls_verify": 0
        },
      },
      "console": {
        "unix": "none",
      },
      "dialer": {
        "std": 1,
      },
    });
    return vc;
  }

  static VpnConfig testHK() {
    var vc = VpnConfig();
    vc.name = "app";
    vc.channel = ".*";
    vc.mode = "auto";
    vc.config = jsonEncode({
      "name": "app",
      "channels": {
        "N0": {
          "remote": "tls://hk.sxbastudio.com:10023",
          "token": "123",
          "tls_verify": 0
        },
      },
      "console": {
        "unix": "none",
      },
      "dialer": {
        "std": 1,
      },
    });
    return vc;
  }

  static VpnConfig testConfig() {
    var vc = VpnConfig();
    vc.name = "app";
    vc.channel = ".*";
    vc.mode = "auto";
    vc.config = "socks5://127.0.0.1:10701";
    return vc;
  }

  @override
  String toString() {
    return 'VpnConfig(name: $name, channel: $channel, state: $state)';
  }
}

class VPN {
  static Future<VpnConfig> load() async {
    try {
      var result = await platform.invokeMethod("vpn.load") as Map;
      var config = VpnConfig();
      if (result.containsKey("name")) {
        config.name = result["name"];
      }
      if (result.containsKey("channel")) {
        config.channel = result["channel"];
      }
      if (result.containsKey("mode")) {
        config.mode = result["mode"];
      }
      if (result.containsKey("config")) {
        config.config = result["config"];
      }
      if (result.containsKey("gfwRules")) {
        config.gfwRules = result["gfwRules"];
      }
      if (result.containsKey("userRules")) {
        config.userRules = result["userRules"];
      }
      config.state = parseVpnState(result["state"]);
      return config;
    } catch (e) {
      _log.info("vpn load fail $e");
      return VpnConfig.error();
    }
  }

  static Future<void> config(VpnConfig config) async {
    await platform.invokeMethod("vpn.config", [
      config.name,
      config.channel,
      config.mode,
      config.config,
      config.gfwRules,
      config.userRules,
    ]);
  }

  static Future<void> start() async {
    await platform.invokeMethod("vpn.start");
  }

  static Future<void> stop() async {
    await platform.invokeMethod("vpn.stop");
  }
}

enum VpnState {
  none,
  error,
  stopped,
  starting,
  running,
  stopping,
}

VpnState parseVpnState(String state) {
  switch (state) {
    case "NONE":
      return VpnState.none;
    case "STOPPED":
      return VpnState.stopped;
    case "STARTING":
      return VpnState.starting;
    case "RUNNING":
      return VpnState.running;
    case "STOPPING":
      return VpnState.stopping;
    default:
      return VpnState.error;
  }
}
