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

// GetFixed returns a list of fixed seed nodes for the given network
func GetFixed(params *chaincfg.Params) []netpkg.Service {
	var filename string
	switch params.Net {
	case chaincfg.MainNetParams.Net:
		filename = "mainnet.txt"
	case chaincfg.TestNet3Params.Net:
		filename = "testnet.txt"
	case chaincfg.SigNetParams.Net:
		filename = "signet.txt"
	default:
		return nil
	}

	path := filepath.Join("data", filename)
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var services []netpkg.Service
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
