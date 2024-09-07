package broadcast

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Maphikza/go-pushtx/internal/handshake"
	netpkg "github.com/Maphikza/go-pushtx/internal/net"
	"github.com/Maphikza/go-pushtx/internal/p2p"
	"github.com/Maphikza/go-pushtx/internal/seeds"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

// Transaction represents a Bitcoin transaction to be broadcast
type Transaction struct {
	Tx *wire.MsgTx
}

// Options represents various options for the broadcast process
type Options struct {
	Network          *chaincfg.Params
	UseTor           TorMode
	FindPeerStrategy FindPeerStrategy
	MaxTime          time.Duration
	DryRun           bool
	TargetPeers      uint8
	UserAgent        *UserAgent
}

// TorMode determines how to use Tor
type TorMode int

const (
	TorBestEffort TorMode = iota
	TorNo
	TorMust
)

// FindPeerStrategy defines how the initial pool of peers is found
type FindPeerStrategy int

const (
	DnsSeedWithFixedFallback FindPeerStrategy = iota
	DnsSeedOnly
	Custom
)

// UserAgent contains custom user agent information
type UserAgent struct {
	Name        string
	Version     uint64
	BlockHeight uint64
}

// Info represents informational messages about the broadcast process
type Info struct {
	Type InfoType
	Data interface{}
}

// InfoType represents the type of information being sent
type InfoType int

const (
	InfoTypeResolvingPeers InfoType = iota
	InfoTypeResolvedPeers
	InfoTypeConnectingToNetwork
	InfoTypeBroadcast
	InfoTypeDone
)

// Report provides information about the broadcast outcome
type Report struct {
	Success map[string]struct{}
	Rejects map[string]string
}

// Broadcast takes a slice of transactions and broadcast options, and returns a channel of Info
func Broadcast(txs []*Transaction, opts *Options) <-chan Info {
	resultChan := make(chan Info)

	go func() {
		defer close(resultChan)

		ctx, cancel := context.WithTimeout(context.Background(), opts.MaxTime)
		defer cancel()

		torProxy := setupTorProxy(opts.UseTor)
		if torProxy == nil && opts.UseTor == TorMust {
			resultChan <- Info{Type: InfoTypeDone, Data: fmt.Errorf("tor proxy required but not found")}
			return
		}

		resultChan <- Info{Type: InfoTypeResolvingPeers}
		addressBook := resolvePeers(opts)
		resultChan <- Info{Type: InfoTypeResolvedPeers, Data: len(addressBook)}

		resultChan <- Info{Type: InfoTypeConnectingToNetwork, Data: torProxy}
		client := p2p.NewClient(torProxy, opts.Network, fmt.Sprintf("%s:%d", opts.UserAgent.Name, opts.UserAgent.Version))
		defer client.Shutdown()

		connectedPeers := connectToPeers(ctx, client, addressBook, opts.TargetPeers)

		broadcastTransactions(ctx, client, connectedPeers, txs, opts.DryRun, resultChan)

		report := generateReport(txs)
		resultChan <- Info{Type: InfoTypeDone, Data: report}
	}()

	return resultChan
}

func setupTorProxy(torMode TorMode) net.Addr {
	if torMode == TorNo {
		return nil
	}

	torProxyStr, err := netpkg.DetectTorProxy()
	if err != nil {
		log.Printf("Failed to detect Tor proxy: %v", err)
		return nil
	}

	// Assuming torProxyStr is in the format "ip:port"
	return &net.TCPAddr{
		IP:   net.ParseIP(torProxyStr[:strings.LastIndex(torProxyStr, ":")]),
		Port: atoi(torProxyStr[strings.LastIndex(torProxyStr, ":")+1:]),
	}
}

// Helper function to convert string to int
func atoi(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

func resolvePeers(opts *Options) []netpkg.Service {
	var addressBook []netpkg.Service

	switch opts.FindPeerStrategy {
	case DnsSeedWithFixedFallback:
		// Start with fixed seeds
		addressBook = seeds.GetFixed(opts.Network)

		// If we don't have enough fixed seeds, add DNS seeds
		if len(addressBook) < 20 {
			addressBook = append(addressBook, seeds.ResolveDNS(opts.Network)...)
		}

	case DnsSeedOnly:
		addressBook = seeds.ResolveDNS(opts.Network)

	case Custom:
		// TODO: Implement custom peer list
		// This could be a place to add user-specified peers or
		// implement any other custom peer discovery logic
		return nil

	default:
		return nil
	}

	// Shuffle the address book to randomize peer selection
	seeds.Shuffle(addressBook)

	return addressBook
}

func connectToPeers(ctx context.Context, client *p2p.Client, addressBook []netpkg.Service, targetPeers uint8) []*handshake.Peer {
	var connectedPeers []*handshake.Peer
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < int(targetPeers) && i < len(addressBook); i++ {
		wg.Add(1)
		go func(service netpkg.Service) {
			defer wg.Done()
			if peer, err := client.Connect(ctx, service); err == nil {
				mu.Lock()
				connectedPeers = append(connectedPeers, peer)
				mu.Unlock()
			}
		}(addressBook[i])
	}

	wg.Wait()
	return connectedPeers
}

func broadcastTransactions(ctx context.Context, client *p2p.Client, peers []*handshake.Peer, txs []*Transaction, dryRun bool, resultChan chan<- Info) {
	for _, tx := range txs {
		for _, peer := range peers {
			if !dryRun {
				err := client.SendTransaction(ctx, peer, tx.Tx)
				if err != nil {
					log.Printf("Failed to send transaction to peer %s: %v", peer.Addr(), err)
					continue
				}
			}
			resultChan <- Info{Type: InfoTypeBroadcast, Data: peer.Addr().String()}
		}
	}
}

func generateReport(txs []*Transaction) *Report {
	report := &Report{
		Success: make(map[string]struct{}),
		Rejects: make(map[string]string),
	}
	for _, tx := range txs {
		report.Success[tx.Tx.TxHash().String()] = struct{}{}
	}
	return report
}
