package p2p

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Maphikza/go-pushtx/internal/handshake"
	netpkg "github.com/Maphikza/go-pushtx/internal/net"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

// Client represents a p2p client
type Client struct {
	network     *chaincfg.Params
	torProxy    net.Addr
	userAgent   string
	peers       map[string]*handshake.Peer
	peersMutex  sync.RWMutex
	messageChan chan Message
	quitChan    chan struct{}
}

// Message represents a message received from a peer
type Message struct {
	Peer    *handshake.Peer
	Payload wire.Message
}

// NewClient creates a new p2p client
func NewClient(torProxy net.Addr, network *chaincfg.Params, userAgent string) *Client {
	return &Client{
		network:     network,
		torProxy:    torProxy,
		userAgent:   userAgent,
		peers:       make(map[string]*handshake.Peer),
		messageChan: make(chan Message, 100),
		quitChan:    make(chan struct{}),
	}
}

// Connect establishes a connection to a peer
func (c *Client) Connect(ctx context.Context, service netpkg.Service) (*handshake.Peer, error) {
	conn, err := c.dialPeer(service)
	if err != nil {
		return nil, fmt.Errorf("failed to dial peer: %w", err)
	}

	peer := handshake.NewPeer(conn)

	localAddr := conn.LocalAddr().(*net.TCPAddr)
	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)

	localNA := wire.NewNetAddress(localAddr, wire.SFNodeNetwork)
	remoteNA := wire.NewNetAddress(remoteAddr, 0)

	versionMsg := wire.NewMsgVersion(localNA, remoteNA, c.generateNonce(), 0)
	versionMsg.UserAgent = c.userAgent

	if err := peer.Handshake(versionMsg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	c.peersMutex.Lock()
	c.peers[service.String()] = peer
	c.peersMutex.Unlock()

	go c.handlePeer(peer)

	return peer, nil
}

// Disconnect closes the connection to a peer
func (c *Client) Disconnect(peer *handshake.Peer) error {
	c.peersMutex.Lock()
	defer c.peersMutex.Unlock()

	addr := peer.Addr().String()
	if _, exists := c.peers[addr]; !exists {
		return fmt.Errorf("peer not found: %s", addr)
	}

	delete(c.peers, addr)
	return peer.Close()
}

// SendTransaction sends a transaction to a peer
func (c *Client) SendTransaction(ctx context.Context, peer *handshake.Peer, tx *wire.MsgTx) error {
	txHash := tx.TxHash()
	invVect := wire.NewInvVect(wire.InvTypeTx, &txHash)
	inv := wire.NewMsgInv()
	inv.AddInvVect(invVect)

	if err := c.sendMessage(peer, inv); err != nil {
		return fmt.Errorf("failed to send inv message: %w", err)
	}

	// Wait for getdata message
	select {
	case msg := <-c.messageChan:
		if msg.Peer == peer {
			getData, ok := msg.Payload.(*wire.MsgGetData)
			if !ok || len(getData.InvList) == 0 || getData.InvList[0].Hash != txHash {
				return fmt.Errorf("unexpected response to inv message")
			}
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	// Send the transaction
	if err := c.sendMessage(peer, tx); err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	return nil
}

// Shutdown closes all peer connections and stops the client
func (c *Client) Shutdown() {
	close(c.quitChan)

	c.peersMutex.Lock()
	defer c.peersMutex.Unlock()

	for _, peer := range c.peers {
		peer.Close()
	}
}

func (c *Client) dialPeer(service netpkg.Service) (net.Conn, error) {
	if c.torProxy != nil {
		dialer, err := netpkg.CreateTorDialer(c.torProxy.String())
		if err != nil {
			return nil, err
		}
		return dialer.Dial("tcp", service.String())
	}
	return net.DialTimeout("tcp", service.String(), 10*time.Second)
}

func (c *Client) handlePeer(peer *handshake.Peer) {
	for {
		msg, err := peer.ReadMessage(c.network.Net)
		if err != nil {
			c.Disconnect(peer)
			return
		}

		select {
		case c.messageChan <- Message{Peer: peer, Payload: msg}:
		case <-c.quitChan:
			return
		}
	}
}

func (c *Client) sendMessage(peer *handshake.Peer, msg wire.Message) error {
	return peer.WriteMessage(msg, c.network.Net)
}

func (c *Client) generateNonce() uint64 {
	// Implement a method to generate a random nonce
	return uint64(time.Now().UnixNano())
}
