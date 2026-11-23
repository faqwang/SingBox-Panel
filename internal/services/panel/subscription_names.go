package panel

import (
	"fmt"
	"strings"

	"github.com/hann0w0/singbox-panel/internal/domain/singbox"
)

func regionFromName(name string) string {
	const base = 0x1F1E6
	runes := []rune(name)
	for i := 0; i+1 < len(runes); i++ {
		a, b := runes[i], runes[i+1]
		if a >= base && a <= base+25 && b >= base && b <= base+25 {
			return string(rune('A'+(a-base))) + string(rune('A'+(b-base)))
		}
	}
	return ""
}

// nameServerNodes supplies names for inbounds without a configured node name.
// A server's only node uses the server name. When it contributes several nodes,
// the protocol or custom inbound tag distinguishes them.
func nameServerNodes(serverName string, nodes []node) {
	serverName = strings.TrimSpace(serverName)
	for i := range nodes {
		if nodes[i].name != "" {
			continue
		}
		if len(nodes) == 1 && serverName != "" {
			nodes[i].name = serverName
			continue
		}
		nodes[i].name = formatNodeDisplayName(serverName, nodes[i].tag, nodes[i].typ)
	}
}

func formatNodeDisplayName(serverName, tag, typ string) string {
	prefix := typ + "-"
	if strings.HasPrefix(tag, prefix) && len(tag) == len(prefix)+6 {
		return fmt.Sprintf("%s - %s", serverName, formatProtocolDisplayName(typ))
	}
	return fmt.Sprintf("%s - %s", serverName, tag)
}

func formatProtocolDisplayName(typ string) string {
	switch typ {
	case "shadowsocks":
		return "Shadowsocks"
	case "socks":
		return "SOCKS5"
	case "http":
		return "HTTP"
	case "vless":
		return "VLESS"
	case "vmess":
		return "VMess"
	case "trojan":
		return "Trojan"
	case "hysteria2":
		return "Hysteria2"
	case "hysteria":
		return "Hysteria"
	case "mixed":
		return "Mixed"
	case "tuic":
		return "TUIC"
	case "anytls":
		return "AnyTLS"
	case "snell":
		return "Snell"
	default:
		return strings.ToUpper(typ)
	}
}

func (n node) getUserIdentity() singbox.ProxyUser {
	identity := n.user
	if identity.Name == "" && identity.Username == "" && identity.UUID == "" && identity.Password == "" {
		identity = n.settings.SingleUserIdentity()
	}
	return identity
}

func (n node) clientNode() singbox.ClientNode {
	return singbox.ClientNode{
		Name:       n.name,
		Server:     n.server,
		ServerPort: n.port,
		Type:       n.typ,
		Settings:   n.settings,
		User:       n.getUserIdentity(),
	}
}

// writeLinks emits one share URI per line (Shadowrocket and friends accept
// plain text; base64 only makes the output unreadable).
