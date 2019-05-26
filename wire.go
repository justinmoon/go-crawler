package main

import (
	"fmt"
	"net"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
)

//type Failure string

// globals
var getaddrSent = make(chan struct{}) // TODO: delete

// q contains addresses we need to visit
var inbox = make(chan *wire.NetAddress)
var online = make([]*peer.Peer, 2)
var offline = make([]*peer.Peer, 2)

// HandleVerack prints when we've received a verack message
func HandleVerack(p *peer.Peer, msg *wire.MsgVerAck) {
	getaddr := wire.NewMsgGetAddr()
	p.QueueMessage(getaddr, getaddrSent)
	<-getaddrSent
	online = append(online, p)
}

// HandleAddr prints when we've received a verack message
// queryDNS loads some initial wire.NetAddress instances
// into the inbox channel
func queryDNS() {
	params := chaincfg.MainNetParams
	defaultRequiredServices := wire.SFNodeNetwork
	connmgr.SeedFromDNS(&params, defaultRequiredServices,
		net.LookupIP, func(addrs []*wire.NetAddress) {
			for _, addr := range addrs {
				inbox <- addr
			}
		})
	<-time.After(time.Second * 10) // HACK!!!
}

func handshake(addr wire.NetAddress) {
	var finished = make(chan bool)
	peerCfg := &peer.Config{
		UserAgentName:    "mooniversity",
		UserAgentVersion: "0.0.1",
		ChainParams:      &chaincfg.MainNetParams,
		Services:         0,
		TrickleInterval:  time.Second * 10,
		Listeners: peer.MessageListeners{
			OnVerAck: HandleVerack,
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				if len(msg.AddrList) > 1 {
					fmt.Printf("received %d addr from %s\n", len(msg.AddrList), addr.IP)
					for _, addr := range msg.AddrList {
						inbox <- addr
					}
				}
				finished <- true
			},
		},
	}
	// hack to format ipv6 strings
	addrString := fmt.Sprintf("[%s]:%d", addr.IP, addr.Port)
	p, err := peer.NewOutboundPeer(peerCfg, addrString)
	if err != nil {
		fmt.Printf("NewOutboundPeer: error %v\n", err)
		offline = append(offline, p)
		//panic("outbound error")
	}

	// Establish the connection to the peer address and mark it connected.
	conn, err := net.Dial("tcp", p.Addr())
	if err != nil {
		fmt.Printf("net.Dial: error %v\n", err)
		offline = append(offline, p)
		//panic("net.Dial error")
		return
	}
	p.AssociateConnection(conn)

	// wait for completion or timeout
	select {
	case <-finished:
		// do nothing
	case <-time.After(time.Second * 3):
		offline = append(offline, p)
	}
	p.Disconnect()
	p.WaitForDisconnect()
}

func worker() {
	for true {
		addr := <-inbox
		handshake(*addr)
	}
}

func collect() {
	for true {
		// print a report
		fmt.Printf("online %d, offline %d, inbox %d\n",
			len(online), len(offline), len(inbox))

		// sleep until next loop
		time.Sleep(1 * time.Second)
	}
}

func main() {
	// launch workers
	for i := 1; i < 1000; i++ {
		go worker()
	}

	// query dns seeds
	go queryDNS()

	// enter status look
	collect()
}
