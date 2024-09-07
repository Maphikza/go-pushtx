package seeds

import (
	"bufio"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	netpkg "github.com/Maphikza/go-pushtx/internal/net"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

var (
	mainnetDNSSeeds = []string{
		"seed.bitcoin.sipa.be",
		"dnsseed.bluematt.me",
		"dnsseed.bitcoin.dashjr.org",
		"seed.bitcoinstats.com",
		"seed.bitcoin.jonasschnelli.ch",
		"seed.btc.petertodd.org",
	}

	testnetDNSSeeds = []string{
		"testnet-seed.bitcoin.jonasschnelli.ch",
		"seed.tbtc.petertodd.org",
		"testnet-seed.bluematt.me",
	}

	// Add known reliable and available nodes
	reliableMainnetNodes = []string{
		"178.128.221.177:8333",
		"74.213.175.99:8333",
		"89.117.19.191:8333",
		"129.80.4.58:8333",
	}

	reliableTestnetNodes = []string{
		"80.114.119.141:18333",
		"74.213.175.99:18333",
		"89.117.19.191:18333",
	}

	reliableSigNetNodes = []string{
		"178.128.221.177:38333",
	}
)

// ResolveDNS resolves DNS seeds for the given network and returns a list of peer addresses
func ResolveDNS(params *chaincfg.Params) []netpkg.Service {
	var dnsSeeds []string
	var defaultPort string

	switch params.Net {
	case wire.MainNet:
		dnsSeeds = mainnetDNSSeeds
		defaultPort = "8333"
	case wire.TestNet3:
		dnsSeeds = testnetDNSSeeds
		defaultPort = "18333"
	default:
		return nil
	}

	var wg sync.WaitGroup
	results := make(chan []netpkg.Service)

	for _, seed := range dnsSeeds {
		wg.Add(1)
		go func(seed string) {
			defer wg.Done()
			addrs, err := net.LookupHost(seed)
			if err != nil {
				return
			}
			var services []netpkg.Service
			for _, addr := range addrs {
				service, err := netpkg.ParseService(net.JoinHostPort(addr, defaultPort))
				if err == nil {
					services = append(services, service)
				}
			}
			results <- services
		}(seed)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var allServices []netpkg.Service
	for services := range results {
		allServices = append(allServices, services...)
	}

	return allServices
}

// GetFixed returns a list of fixed seed nodes for the given network, starting with known reliable nodes
func GetFixed(params *chaincfg.Params) []netpkg.Service {
	var filename string
	var reliableNodes []string

	switch params.Net {
	case chaincfg.MainNetParams.Net:
		filename = "mainnet.txt"
		reliableNodes = reliableMainnetNodes
	case chaincfg.TestNet3Params.Net:
		filename = "testnet.txt"
		reliableNodes = reliableTestnetNodes
	case chaincfg.SigNetParams.Net:
		filename = "signet.txt"
		reliableNodes = reliableSigNetNodes
	default:
		return nil
	}

	var services []netpkg.Service

	// First, add the reliable nodes
	for _, addr := range reliableNodes {
		service, err := netpkg.ParseService(addr)
		if err == nil {
			services = append(services, service)
		}
	}

	// Then, add nodes from the file
	path := filepath.Join("data", filename)
	file, err := os.Open(path)
	if err != nil {
		return services // Return the reliable nodes if file can't be opened
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		addr := strings.TrimSpace(scanner.Text())
		if addr == "" || strings.HasPrefix(addr, "#") {
			continue
		}
		service, err := netpkg.ParseService(addr)
		if err == nil {
			services = append(services, service)
		}
	}

	return services
}

// Shuffle randomly shuffles the order of services in the slice
func Shuffle(services []netpkg.Service) {
	r := rand.New(rand.NewSource(rand.Int63()))
	r.Shuffle(len(services), func(i, j int) {
		services[i], services[j] = services[j], services[i]
	})
}
