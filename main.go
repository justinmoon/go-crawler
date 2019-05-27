package main

import (
	"fmt"
	"net"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/deckarep/golang-set"
)

// globals

// q contains addresses we need to visit
var inbox = make(chan *wire.NetAddress, 10000000)

//var inbox = queue.New()
//var online = make([]*peer.Peer, 2)
var online = mapset.NewSet()
var offline = mapset.NewSet()

// HandleVerack prints when we've received a verack message

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

func NAToS(na *wire.NetAddress) string {
	return fmt.Sprintf("[%s]:%d", na.IP, na.Port)
}

func visit(addr *wire.NetAddress) {
	var finished = make(chan bool)
	var handshakeSuccessful = false
	peerCfg := &peer.Config{
		UserAgentName:    "mooniversity",
		UserAgentVersion: "0.0.1",
		ChainParams:      &chaincfg.MainNetParams,
		Services:         0,
		TrickleInterval:  time.Second * 10,
		Listeners: peer.MessageListeners{
			OnVerAck: func(p *peer.Peer, msg *wire.MsgVerAck) {
				getaddr := wire.NewMsgGetAddr()
				getaddrSent := make(chan struct{}) // TODO: delete
				p.QueueMessage(getaddr, getaddrSent)
				<-getaddrSent
				online.Add(p.NA().IP.String())
				handshakeSuccessful = true
			},
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				if len(msg.AddrList) > 1 {
					//fmt.Printf("received %d addr from %s\n", len(msg.AddrList), NAToS(addr))
					for _, addr := range msg.AddrList {
						inbox <- addr
					}
					finished <- true
				}
			},
		},
	}

	// create peer.Peer instance
	p, err := peer.NewOutboundPeer(peerCfg, NAToS(addr))
	if err != nil {
		fmt.Printf("NewOutboundPeer: %v\n", err)
		return
	}

	// give peer.Peer instance a tcp connection
	d := net.Dialer{Timeout: time.Second}
	conn, err := d.Dial("tcp", p.Addr())
	if err != nil {
		fmt.Printf("net.Dial: %v\n", err)
		offline.Add(p.NA().IP.String())
		return
	}
	p.AssociateConnection(conn)

	// wait for completion or timeout
	//fmt.Println("connected")
	select {
	case <-finished:
	case <-time.After(time.Second * 3):
		if !handshakeSuccessful {
			fmt.Println("timeout")
			offline.Add(p.NA().IP.String())
		}
	}

	p.Disconnect()
	p.WaitForDisconnect()
	//fmt.Println("disconnected\n")
}

func worker() {
	for true {
		addr := <-inbox
		for {
			isNew := !(online.Contains(addr.IP.String()) ||
				online.Contains(addr.IP.String()))
			if isNew {
				break
			}
			addr = <-inbox
		}
		visit(addr)
	}
}

func collect() {
	nOnline := 0
	incOnline := 0
	nOffline := 0
	incOffline := 0
	for true {
		offlineSlice := offline.ToSlice()
		onlineSlice := online.ToSlice()
		incOnline = len(onlineSlice) - nOnline
		incOffline = len(offlineSlice) - nOffline
		// print a report
		fmt.Printf("online %d (%d), offline %d (%d), inbox %d\n",
			len(onlineSlice), incOnline, len(offlineSlice), incOffline, len(inbox))
		nOnline = len(onlineSlice)
		nOffline = len(offlineSlice)

		// sleep until next loop
		time.Sleep(10 * time.Second)
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
