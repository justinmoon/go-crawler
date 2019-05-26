package main

import (
	"fmt"
	"net"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"

	"github.com/golang-collections/go-datastructures/queue"
)

//type Failure string

// globals
var connected = make(chan struct{})
var addrsReceived = make(chan []*wire.NetAddress)

//var connectionFailed = make(chan []*wire.NetAddress)

var getaddrSent = make(chan struct{}) // TODO: delete

// q contains addresses we need to visit
var q = queue.New(int64(10000000))

// online is the addresses we've successfully visited
var online = make([]*peer.Peer, 1000)

// offline is the addresses we weren't able to visit
var offline = make([]*peer.Peer, 1000)

// HandleVersion prints when we've received a version message
func HandleVersion(p *peer.Peer, msg *wire.MsgVersion) *wire.MsgReject {
	fmt.Println("received version")
	return nil
}

// HandleVerack prints when we've received a verack message
func HandleVerack(p *peer.Peer, msg *wire.MsgVerAck) {
	fmt.Println("received verack")
	//connected <- struct{}{}
	getaddr := wire.NewMsgGetAddr()
	p.QueueMessage(getaddr, getaddrSent)
	<-getaddrSent
	fmt.Println("sent getaddr")
}

// HandleAddr prints when we've received a verack message
func HandleAddr(p *peer.Peer, msg *wire.MsgAddr) {
	fmt.Printf("received %d addr\n", len(msg.AddrList))
	if len(msg.AddrList) > 1 {
		addrsReceived <- msg.AddrList
	}
}

// queryDNS loads some initial wire.NetAddress instances
// into the queue
func queryDNS(q *queue.Queue) {
	params := chaincfg.MainNetParams
	defaultRequiredServices := wire.SFNodeNetwork
	connmgr.SeedFromDNS(&params, defaultRequiredServices,
		net.LookupIP, func(addrs []*wire.NetAddress) {
			fmt.Println("dns addrs: ", addrs)
			for _, addr := range addrs {
				q.Put(*addr)
			}
		})
	<-time.After(time.Second * 10)
}

func handshake(addr wire.NetAddress) *peer.Peer {
	peerCfg := &peer.Config{
		UserAgentName:    "mooniversity",
		UserAgentVersion: "0.0.1",
		ChainParams:      &chaincfg.MainNetParams,
		Services:         0,
		TrickleInterval:  time.Second * 10,
		Listeners: peer.MessageListeners{
			OnVersion: HandleVersion,
			OnVerAck:  HandleVerack,
			OnAddr:    HandleAddr,
		},
	}
	//addr := "92.106.102.25:8333"
	//addr := "195.168.36.20:8333"
	//addr := "109.239.79.181:8333"
	a := fmt.Sprintf("%s:%d", addr.IP, addr.Port)
	p, err := peer.NewOutboundPeer(peerCfg, a)
	if err != nil {
		fmt.Printf("NewOutboundPeer: error %v\n", err)
		panic("outbound error")
	}

	// Establish the connection to the peer address and mark it connected.
	fmt.Println("starting tcp connection")
	conn, err := net.Dial("tcp", p.Addr())
	fmt.Println("tcp connection established")
	if err != nil {
		fmt.Printf("net.Dial: error %v\n", err)
		panic("net.Dial error")
	}
	p.AssociateConnection(conn)
	fmt.Println("finished AssociateConnection")

	// Wait for the verack message or timeout in case of failure.
	select {
	//case <-connected:
	//return p
	case addrs := <-addrsReceived:
		q.Put(addrs)
	case <-time.After(time.Second * 10):
		fmt.Println("verack timeout")
		// Disconnect the peer.
		p.Disconnect()
		p.WaitForDisconnect()
		panic("verack timeout")
	}

	fmt.Println("queue size: ", q.Len())
	top, err := q.Get(1)
	fmt.Println("queue get: ", top)
	return p
}

func main() {
	queryDNS(q)
	addrInterface, _ := q.Get(1) // returns some stupid interface
	first := addrInterface[0]
	peer := handshake(first.(wire.NetAddress))
	fmt.Println("peer created: ", peer)
}
