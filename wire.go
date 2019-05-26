package main

import (
	"fmt"
	"net"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/sheerun/queue"
)

// globals

// q contains addresses we need to visit
//var inbox = make(chan *wire.NetAddress)
var inbox = queue.New()
var online = make([]*peer.Peer, 2)
var offline = make([]*peer.Peer, 2)

// HandleVerack prints when we've received a verack message

// queryDNS loads some initial wire.NetAddress instances
// into the inbox channel
func queryDNS() {
	params := chaincfg.MainNetParams
	defaultRequiredServices := wire.SFNodeNetwork
	connmgr.SeedFromDNS(&params, defaultRequiredServices,
		net.LookupIP, func(addrs []*wire.NetAddress) {
			for _, addr := range addrs {
				inbox.Append(NAToS(addr))
			}
		})
	<-time.After(time.Second * 10) // HACK!!!
}

func NAToS(na *wire.NetAddress) string {
	return fmt.Sprintf("[%s]:%d", na.IP, na.Port)
}

func visit(a string) {
	var finished = make(chan bool)
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
				online = append(online, p)
			},
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				if len(msg.AddrList) > 1 {
					fmt.Printf("received %d addr from %s\n", len(msg.AddrList), a)
					for _, addr := range msg.AddrList {
						inbox.Append(NAToS(addr))
					}
				}
				finished <- true
			},
		},
	}

	// create peer.Peer instance
	p, err := peer.NewOutboundPeer(peerCfg, a)
	if err != nil {
		fmt.Printf("NewOutboundPeer: error %v\n", err)
		return
	}

	// give peer.Peer instance a tcp connection
	conn, err := net.Dial("tcp", p.Addr())
	if err != nil {
		fmt.Printf("net.Dial: error %v\n", err)
		offline = append(offline, p)
		return
	}
	p.AssociateConnection(conn)

	// wait for completion or timeout
	select {
	case <-finished:
	case <-time.After(time.Second * 3):
		offline = append(offline, p)
	}
	p.Disconnect()
	p.WaitForDisconnect()
}

func worker() {
	counter := 1
	for true {
		addr := inbox.Pop()
		visit(addr.(string))
		fmt.Println(counter)
		counter++
	}
}

func collect() {
	for true {
		// print a report
		fmt.Printf("online %d, offline %d, inbox %d\n",
			len(online), len(offline), inbox.Length())

		// sleep until next loop
		time.Sleep(1 * time.Second)
	}
}

func main() {
	// launch workers
	for i := 1; i < 500; i++ {
		go worker()
	}

	// query dns seeds
	go queryDNS()

	// enter status look
	collect()
}
