//
//  PacketTunnelProvider.swift
//  Proxy
//
//  Created by cover on 2023/5/30.
//
import Bsrouter
import Foundation
import NetworkExtension

enum MessageError: Error {
    case runtimeError(String?)
}

func getDocumentsDirectory() -> String {
    let paths = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)
    let documentsDirectory = paths[0]
    return documentsDirectory.path()
}

extension Data {
    var hexDescription: String {
        return reduce("") { $0 + String(format: "%02x", $1) }
    }
}

class PacketTunnelProvider: NEPacketTunnelProvider, BsrouterLoggerProtocol, BsrouterSenderProtocol {
    var netAddr = "10.7.3.7"
    var netPrefix = "24"
    var netMask = "255.255.255.0"
    var gwAddr = "10.7.3.1"
    var gwDNS = "10.7.3.1"
    var mtu = 1500
    var nodeConfig = ""
    var gfwRules = ""
    var userRules = ""
    var channel = ""
    var mode = ""
    var excluded: [NEIPv4Route] = []

    var gatewayIn: BsrouterSenderProtocol?

    override init() {
        super.init()
        #if os(iOS) || os(watchOS) || os(tvOS)
            let dir = getDocumentsDirectory()
            BsrouterBootstrap(dir, self)
        #else
            let dir = FileManager.default.homeDirectoryForCurrentUser.path()
            BsrouterBootstrap(dir, self)
        #endif
    }

    func printLog(_ line: String?) {
        NSLog("%@", line!)
    }

    func done() {}

    func send(_ p0: Data?) {
        packetFlow.writePackets([p0!], withProtocols: [AF_INET as NSNumber])
    }

    func readFlowData() {
        packetFlow.readPackets { (packets: [Data], protocols: [NSNumber]) in
            for (i, packet) in packets.enumerated() {
                if protocols[i].intValue != AF_INET {
                    continue
                }
                self.gatewayIn?.send(packet)
            }
            self.readFlowData()
        }
    }

    func startNode() -> BsrouterResultProtocol? {
//        #if os(iOS) || os(watchOS) || os(tvOS)
            var res = BsrouterStartNode(nodeConfig)
//        #else
//            var res = BsrouterStartConsole(nodeConfig)
//        #endif
        if res?.code() == 0 {
            excluded = []
            res?.stringValue().split(separator: ",").forEach { ip in
                if ip.isEmpty {
                    return
                }
                if ip.contains(".") {
                    excluded.append(NEIPv4Route(destinationAddress: String(ip), subnetMask: "255.255.255.255"))
                }
            }
            res = BsrouterStartGateway(netAddr, gwAddr, gwDNS, channel, mode)
        }
        return res
    }

    func stopNode() {
        BsrouterStopGateway()
        BsrouterStopNode()
    }

    override func startTunnel(options _: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        let protocolConfiguration = self.protocolConfiguration as? NETunnelProviderProtocol
        let pconf = protocolConfiguration?.providerConfiguration
        nodeConfig = pconf!["config"] as! String
        gfwRules = pconf!["gfwRules"] as! String
        userRules = pconf!["userRules"] as! String
        channel = pconf!["channel"] as! String
        mode = pconf!["mode"] as! String

        var res: BsrouterResultProtocol?

        res = BsrouterSetupGFW(gfwRules, userRules)
        if res?.code() != 0 {
            completionHandler(MessageError.runtimeError(res?.message()))
            return
        }

        gatewayIn = BsrouterSetupPipeDevice(self)

        res = startNode()
        if res?.code() != 0 {
            completionHandler(MessageError.runtimeError(res?.message()))
            return
        }

        let newSettings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: gwAddr)
        newSettings.mtu = mtu as NSNumber
        newSettings.ipv4Settings = NEIPv4Settings(addresses: [netAddr], subnetMasks: [netMask])
        newSettings.ipv4Settings?.includedRoutes = [NEIPv4Route.default()]
        if !excluded.isEmpty {
            newSettings.ipv4Settings?.excludedRoutes = excluded
        }
        newSettings.dnsSettings = NEDNSSettings(servers: [gwDNS])
        setTunnelNetworkSettings(newSettings) { e in
            completionHandler(e)
            self.readFlowData()
        }
    }

    override func stopTunnel(with _: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        stopNode()
        completionHandler()
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        let res = BsrouterHandleMessage(messageData)
        if let handler = completionHandler {
            handler(res)
        }
    }

    override func sleep(completionHandler: @escaping () -> Void) {
        stopNode()
        completionHandler()
    }

    override func wake() {
        _ = startNode()
    }
}
