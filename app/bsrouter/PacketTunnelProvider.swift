import Darwin
//
//  PacketTunnelProvider.swift
//  Proxy
//
//  Created by cover on 2023/5/30.
//
import Foundation
import NetworkExtension
import Bsrouter

//class Logger : NSObject,BsrouterLoggerProtocol{
//    func print(_ line: String?) {
//        NSLog("%@", line ?? "nil");
//    }
//}

func getDocumentsDirectory() -> String {
    let paths = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)
    let documentsDirectory = paths[0]
    return documentsDirectory.path()
}

class PacketTunnelProvider: NEPacketTunnelProvider,BsrouterSenderProtocol,BsrouterLoggerProtocol {
    var tun_ip = "10.7.3.7"
    var tun_gw = "10.7.3.1"
    var tun_pref = "24"
    var tun_mask = "255.255.255.0"
    var tun_dns = "10.7.3.1"
    var tun_mtu = 1500
    var config = ""
    
    var gatewayIn: BsrouterSenderProtocol?
    
    override init() {
        super.init();
//        BsrouterBootstrap(<#T##dir: String?##String?#>, <#T##log: BsrouterLoggerProtocol?##BsrouterLoggerProtocol?#>)
#if os(iOS) || os(watchOS) || os(tvOS)
        var dir = getDocumentsDirectory()
        BsrouterBootstrap(dir, self)
#elseif os(macOS)
        var dir = FileManager.default.homeDirectoryForCurrentUser.path()
        BsrouterBootstrap(dir, self)
#else
        var dir = FileManager.default.homeDirectoryForCurrentUser.path()
        BsrouterBootstrap(dir, self)
#endif
    }
    
    
    func printLog(_ line: String?) {
        NSLog("%@", line!);
    }
    
    func done() {
    }
    
    func send(_ p0: Data?) {
        self.packetFlow.writePackets([p0!], withProtocols: [AF_INET as NSNumber])
    }

    func readFlowData() {
        self.packetFlow.readPackets { (packets: [Data], protocols: [NSNumber]) in
            for (i, packet) in packets.enumerated() {
                if protocols[i].intValue != AF_INET {
                    continue
                }
                self.gatewayIn?.send(packet)
            }
            self.readFlowData()
        }
    }

    func startNode() {
    }
    
    func stopNode(){
        
    }

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        let protocolConfiguration = self.protocolConfiguration as? NETunnelProviderProtocol
        let config = protocolConfiguration?.providerConfiguration
        self.config = config!["config"] as! String
        let newSettings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: self.tun_gw)
        newSettings.mtu = self.tun_mtu as NSNumber
        newSettings.ipv4Settings = NEIPv4Settings(addresses: [self.tun_ip], subnetMasks: [self.tun_mask])
        newSettings.ipv4Settings?.includedRoutes = [NEIPv4Route.default()]
        newSettings.dnsSettings = NEDNSSettings(servers: [self.tun_dns])
        setTunnelNetworkSettings(newSettings) { e in
            self.startNode()
            completionHandler(e)
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        self.stopNode()
        completionHandler()
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        // Add code here to handle the message.
        if let handler = completionHandler {
            handler(messageData)
        }
    }

    override func sleep(completionHandler: @escaping () -> Void) {
        // Add code here to get ready to sleep.
        completionHandler()
    }

    override func wake() {
        // Add code here to wake up.
    }
}
