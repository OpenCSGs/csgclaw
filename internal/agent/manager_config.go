package agent

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/runtime/picoclawsandbox"
)

const managerAgentsDirName = "agents"

type localIPDetector struct {
	listInterfaces func() ([]net.Interface, error)
	interfaceAddrs func(iface net.Interface) ([]net.Addr, error)
	dialUDP4       func() (net.Conn, error)
}

var defaultLocalIPDetector = localIPDetector{
	listInterfaces: net.Interfaces,
	interfaceAddrs: func(iface net.Interface) ([]net.Addr, error) {
		return iface.Addrs()
	},
	dialUDP4: func() (net.Conn, error) {
		return net.Dial("udp4", "8.8.8.8:80")
	},
}

func ensureManagerPicoClawConfig(server config.ServerConfig, model config.ModelConfig) (string, error) {
	return ensureAgentPicoClawConfig(ManagerName, "u-manager", server, model)
}

func ensureAgentPicoClawConfig(agentName, botID string, server config.ServerConfig, model config.ModelConfig) (string, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return "", err
	}
	return picoclawsandbox.EnsureConfig(agentHome, botID, server, model, resolveManagerBaseURL)
}

func managerPicoClawRoot() (string, error) {
	return agentPicoClawRoot(ManagerName)
}

func agentWorkspacePicoClawConfigRoot(agentName string) (string, error) {
	return agentPicoClawRoot(agentName)
}

func agentPicoClawRoot(agentName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	return picoclawsandbox.Root(filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName)), nil
}

func renderManagerPicoClawConfig(server config.ServerConfig, model config.ModelConfig) ([]byte, error) {
	return renderAgentPicoClawConfig("u-manager", server, model)
}

func renderAgentPicoClawConfig(botID string, server config.ServerConfig, model config.ModelConfig) ([]byte, error) {
	return picoclawsandbox.RenderConfig(botID, server, model, resolveManagerBaseURL)
}

func picoclawBridgeModelID(modelID string) string {
	return picoclawsandbox.BridgeModelID(modelID)
}

func resolveManagerBaseURL(server config.ServerConfig) string {
	if server.AdvertiseBaseURL != "" {
		baseURL := strings.TrimRight(server.AdvertiseBaseURL, "/")
		slog.Debug("local ip detector using advertise_base_url", "base_url", baseURL)
		return baseURL
	}
	port := config.ListenPort(server.ListenAddr)
	if ip := localIPv4Resolver(); ip != "" {
		baseURL := fmt.Sprintf("http://%s:%s", ip, port)
		slog.Debug("local ip detector resolved manager base url", "ip", ip, "port", port, "base_url", baseURL)
		return baseURL
	}
	slog.Debug("local ip detector could not resolve manager base url", "listen_addr", server.ListenAddr, "port", port)
	return ""
}

func localIPv4() string {
	return defaultLocalIPDetector.localIPv4()
}

func (d localIPDetector) localIPv4() string {
	if ip := d.interfaceIPv4(); ip != "" {
		slog.Debug("local ip detector selected interface address", "ip", ip)
		return ip
	}
	slog.Debug("local ip detector did not find suitable interface address; trying outbound probe")
	ip := d.outboundIPv4()
	if ip != "" {
		slog.Debug("local ip detector selected outbound probe address", "ip", ip)
		return ip
	}
	slog.Debug("local ip detector found no usable ipv4 address")
	return ""
}

func (d localIPDetector) outboundIPv4() string {
	if d.dialUDP4 == nil {
		slog.Debug("local ip detector outbound probe unavailable: dial function is nil")
		return ""
	}
	conn, err := d.dialUDP4()
	if err != nil {
		slog.Debug("local ip detector outbound probe failed", "error", err)
		return ""
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		slog.Debug("local ip detector outbound probe returned unexpected local address", "addr", conn.LocalAddr())
		return ""
	}
	ip := addr.IP.To4()
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
		slog.Debug("local ip detector rejected outbound probe address", "ip", addr.IP.String())
		return ""
	}
	slog.Debug("local ip detector outbound probe candidate", "ip", ip.String())
	return ip.String()
}

func (d localIPDetector) interfaceIPv4() string {
	if d.listInterfaces == nil || d.interfaceAddrs == nil {
		slog.Debug("local ip detector interface scan unavailable", "has_list_interfaces", d.listInterfaces != nil, "has_interface_addrs", d.interfaceAddrs != nil)
		return ""
	}
	ifaces, err := d.listInterfaces()
	if err != nil {
		slog.Debug("local ip detector failed to list interfaces", "error", err)
		return ""
	}
	bestFallback := ""
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			slog.Debug("local ip detector skipping interface", "interface", iface.Name, "reason", "down", "flags", iface.Flags.String())
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			slog.Debug("local ip detector skipping interface", "interface", iface.Name, "reason", "loopback", "flags", iface.Flags.String())
			continue
		}
		if iface.Flags&net.FlagPointToPoint != 0 {
			slog.Debug("local ip detector skipping interface", "interface", iface.Name, "reason", "point_to_point", "flags", iface.Flags.String())
			continue
		}
		addrs, err := d.interfaceAddrs(iface)
		if err != nil {
			slog.Debug("local ip detector failed to list interface addresses", "interface", iface.Name, "error", err)
			continue
		}
		for _, addr := range addrs {
			ip := ipv4FromAddr(addr)
			if ip == "" {
				slog.Debug("local ip detector rejected interface address", "interface", iface.Name, "addr", addr.String(), "reason", "not_usable_ipv4")
				continue
			}
			parsed := net.ParseIP(ip)
			if parsed == nil {
				slog.Debug("local ip detector rejected interface address", "interface", iface.Name, "addr", addr.String(), "reason", "parse_failed")
				continue
			}
			if parsed.IsPrivate() {
				slog.Debug("local ip detector selected private interface address", "interface", iface.Name, "ip", ip)
				return ip
			}
			if bestFallback == "" {
				slog.Debug("local ip detector recorded fallback interface address", "interface", iface.Name, "ip", ip)
				bestFallback = ip
			}
		}
	}
	if bestFallback != "" {
		slog.Debug("local ip detector selected fallback interface address", "ip", bestFallback)
	}
	return bestFallback
}

func ipv4FromAddr(addr net.Addr) string {
	switch v := addr.(type) {
	case *net.IPNet:
		ip := v.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	case *net.IPAddr:
		ip := v.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	default:
		return ""
	}
}

func renderManagerSecurityConfig(server config.ServerConfig, model config.ModelConfig) string {
	return picoclawsandbox.RenderSecurityConfig(server, model)
}
