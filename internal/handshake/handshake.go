package handshake

import (
	"errors"
	"net"
	"sync"

	"github.com/btcsuite/btcd/wire"
)

// Peer represents a connected peer
type Peer struct {
	conn           net.Conn
	versionNonce   uint64
	services       wire.ServiceFlag
	userAgent      string
	verAckReceived bool
	mu             sync.Mutex
}

// NewPeer creates a new Peer instance
func NewPeer(conn net.Conn) *Peer {
	return &Peer{
		conn: conn,
	}
}

// Handshake performs the Bitcoin protocol handshake
func (p *Peer) Handshake(localVersion *wire.MsgVersion) error {
	if err := p.sendVersion(localVersion); err != nil {
		return err
	}

	if err := p.readVersion(); err != nil {
		return err
	}

	if err := p.sendVerAck(); err != nil {
		return err
	}

	if err := p.readVerAck(); err != nil {
		return err
	}

	return nil
}

func (p *Peer) sendVersion(localVersion *wire.MsgVersion) error {
	return wire.WriteMessage(p.conn, localVersion, wire.ProtocolVersion, wire.MainNet)
}

func (p *Peer) readVersion() error {
	msg, _, err := wire.ReadMessage(p.conn, wire.ProtocolVersion, wire.MainNet)
	if err != nil {
		return err
	}

	version, ok := msg.(*wire.MsgVersion)
	if !ok {
		return errors.New("expected version message")
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.versionNonce = version.Nonce
	p.services = version.Services
	p.userAgent = version.UserAgent

	return nil
}

func (p *Peer) sendVerAck() error {
	verAck := wire.NewMsgVerAck()
	return wire.WriteMessage(p.conn, verAck, wire.ProtocolVersion, wire.MainNet)
}

func (p *Peer) readVerAck() error {
	msg, _, err := wire.ReadMessage(p.conn, wire.ProtocolVersion, wire.MainNet)
	if err != nil {
		return err
	}

	_, ok := msg.(*wire.MsgVerAck)
	if !ok {
		return errors.New("expected verack message")
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.verAckReceived = true

	return nil
}

// IsHandshakeComplete checks if the handshake process is complete
func (p *Peer) IsHandshakeComplete() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.verAckReceived
}

// Addr returns the network address of the peer
func (p *Peer) Addr() net.Addr {
	return p.conn.RemoteAddr()
}

// Services returns the services supported by the peer
func (p *Peer) Services() wire.ServiceFlag {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.services
}

// UserAgent returns the user agent string of the peer
func (p *Peer) UserAgent() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.userAgent
}

// Close closes the connection to the peer
func (p *Peer) Close() error {
	return p.conn.Close()
}

func (p *Peer) ReadMessage(btcNet wire.BitcoinNet) (wire.Message, error) {
	msg, _, err := wire.ReadMessage(p.conn, wire.ProtocolVersion, btcNet)
	return msg, err
}

func (p *Peer) WriteMessage(msg wire.Message, btcNet wire.BitcoinNet) error {
	return wire.WriteMessage(p.conn, msg, wire.ProtocolVersion, btcNet)
}
