package agent

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/runtime/picoclawsandbox"
)

const managerAgentsDirName = "agents"

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
	workspaceRoot, err := agentWorkspaceRoot(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(workspaceRoot, filepath.FromSlash(picoclawsandbox.HostPicoClawStateDir)), nil
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
		return strings.TrimRight(server.AdvertiseBaseURL, "/")
	}
	port := config.ListenPort(server.ListenAddr)
	if ip := localIPv4Resolver(); ip != "" {
		return fmt.Sprintf("http://%s:%s", ip, port)
	}
	return ""
}

func localIPv4() string {
	if ip := outboundIPv4(); ip != "" {
		return ip
	}
	return interfaceIPv4()
}

func outboundIPv4() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return ""
	}
	ip := addr.IP.To4()
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
		return ""
	}
	return ip.String()
}

func interfaceIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ip := ipv4FromAddr(addr); ip != "" {
				return ip
			}
		}
	}
	return ""
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
