package net

import (
	"encoding/base32"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"golang.org/x/net/proxy"
)

// Network represents the type of network
type Network int

const (
	// IPv4 network
	IPv4 Network = iota
	// IPv6 network
	IPv6
	// TorV3 network (Onion v3)
	TorV3
)

// Address represents a network address
type Address struct {
	network Network
	data    []byte
}

// Service represents a combination of Address and port
type Service struct {
	addr net.Addr
}

// NewService creates a new Service from a net.Addr
func NewService(addr net.Addr) Service {
	return Service{addr: addr}
}

func (s Service) Network() string {
	return s.addr.Network()
}

func (s Service) String() string {
	return s.addr.String()
}

// NewIPv4Address creates a new IPv4 address
func NewIPv4Address(ip net.IP) Address {
	return Address{
		network: IPv4,
		data:    ip.To4(),
	}
}

// NewIPv6Address creates a new IPv6 address
func NewIPv6Address(ip net.IP) Address {
	return Address{
		network: IPv6,
		data:    ip.To16(),
	}
}

// NewTorV3Address creates a new Tor v3 address
func NewTorV3Address(onionAddress string) (Address, error) {
	if !strings.HasSuffix(onionAddress, ".onion") {
		return Address{}, errors.New("invalid onion address")
	}

	// Remove the .onion suffix and decode the base32 encoding
	onionAddress = strings.TrimSuffix(onionAddress, ".onion")
	data, err := base32Decode(onionAddress)
	if err != nil {
		return Address{}, err
	}

	return Address{
		network: TorV3,
		data:    data,
	}, nil
}

// Network returns the network type of the address
func (a Address) Network() string {
	switch a.network {
	case IPv4:
		return "ipv4"
	case IPv6:
		return "ipv6"
	case TorV3:
		return "torv3"
	default:
		return "unknown"
	}
}

// String returns a string representation of the address
func (a Address) String() string {
	switch a.network {
	case IPv4, IPv6:
		return net.IP(a.data).String()
	case TorV3:
		return fmt.Sprintf("%s.onion", base32Encode(a.data))
	default:
		return "unknown"
	}
}

// ParseService parses a string into a Service
func ParseService(s string) (Service, error) {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return Service{}, err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return Service{}, err
	}

	ip := net.ParseIP(host)
	if ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return NewService(&net.TCPAddr{IP: ip4, Port: int(port)}), nil
		}
		return NewService(&net.TCPAddr{IP: ip, Port: int(port)}), nil
	}

	if strings.HasSuffix(host, ".onion") {
		addr, err := NewTorV3Address(host)
		if err != nil {
			return Service{}, err
		}
		return NewService(&customAddr{addr: addr, port: uint16(port)}), nil
	}

	return Service{}, errors.New("invalid address")
}

// DetectTorProxy attempts to detect a local Tor proxy
func DetectTorProxy() (proxyAddr string, err error) {
	ports := []string{"9050", "9150"} // Common Tor SOCKS proxy ports

	for _, port := range ports {
		addr := "127.0.0.1:" + port
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			return addr, nil
		}
	}

	return "", errors.New("tor proxy not found")
}

// CreateTorDialer creates a proxy.Dialer for Tor connections
func CreateTorDialer(proxyAddr string) (proxy.Dialer, error) {
	return proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
}

// Helper functions for base32 encoding/decoding
func base32Encode(data []byte) string {
	return base32.StdEncoding.EncodeToString(data)
}

func base32Decode(s string) ([]byte, error) {
	return base32.StdEncoding.DecodeString(s)
}

// customAddr is a helper type to implement net.Addr for TorV3 addresses
type customAddr struct {
	addr Address
	port uint16
}

func (ca *customAddr) Network() string {
	return ca.addr.Network()
}

func (ca *customAddr) String() string {
	return fmt.Sprintf("%s:%d", ca.addr.String(), ca.port)
}
