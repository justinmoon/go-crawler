// - keep nodes in a map
// - `inbox` is the ip's of nodes that need to be visited
// 	 when we see a new node it's IP goes in `inbox`
// - goroutine every 10 minutes looking for nodes to visit
//   (LastVisitMissed < Time.minute * 10 ** VisitsMissed
// - TODO: make a separate db folder for the `nodes` stuff

package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
)

var lock = sync.RWMutex{}

// Node represents a bitcoin peer
type Node struct {
	IP              string
	Port            int
	VisitsMissed    int
	LastVisitMissed int
}

func writeNode(node Node) {
	lock.Lock()
	defer lock.Unlock()
	nodes[node.IP] = node
}

func readNode(ip string) (Node, bool) {
	lock.RLock()
	defer lock.RUnlock()
	node, ok := nodes[ip]
	return node, ok
}

func addNodes(AddrList []*wire.NetAddress) {
	for _, addr := range AddrList {
		ip := addr.IP.String()
		if _, ok := readNode(ip); !ok {
			node := Node{ip, int(addr.Port), -1, -1}
			writeNode(node)
			inbox <- node
		}
	}
}

func onlineNodes() []Node {
	lock.RLock()
	defer lock.RUnlock()
	n := make([]Node, 0)
	for _, node := range nodes {
		if node.VisitsMissed == 0 {
			n = append(n, node)
		}
	}
	return n
}

func offlineNodes() []Node {
	lock.RLock()
	defer lock.RUnlock()
	n := make([]Node, 0)
	for _, node := range nodes {
		if node.VisitsMissed > 0 {
			n = append(n, node)
		}
	}
	return n
}

func untouchedNodes() []Node {
	lock.RLock()
	defer lock.RUnlock()
	n := make([]Node, 0)
	for _, node := range nodes {
		if node.VisitsMissed == -1 {
			n = append(n, node)
		}
	}
	return n
}

func missVisit(node Node) {
	node.VisitsMissed = 1
	//if node.VisitsMissed == -1 {
	//node.VisitsMissed = 1
	//} else {
	//node.VisitsMissed++
	//}
	// TODO: update timestamp, too
	writeNode(node)
}

func makeVisit(node Node) {
	node.VisitsMissed = 0
	writeNode(node)
}

var nodes = make(map[string]Node)

// q contains addresses we need to visit
var inbox = make(chan Node, 10000000)

// HandleVerack prints when we've received a verack message

// queryDNS loads some initial wire.NetAddress instances
// into the inbox channel
func queryDNS() {
	params := chaincfg.MainNetParams
	defaultRequiredServices := wire.SFNodeNetwork
	connmgr.SeedFromDNS(&params, defaultRequiredServices,
		net.LookupIP, addNodes)
	<-time.After(time.Second * 10) // HACK!!!
}

func NToS(node Node) string {
	return fmt.Sprintf("[%s]:%d", node.IP, node.Port)
}

func visit(node *Node) {
	var finished = make(chan bool)
	//var failed = make(chan bool)
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
				makeVisit(*node)
				handshakeSuccessful = true
			},
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				if len(msg.AddrList) > 1 {
					//fmt.Printf("received %d addr from %s\n", len(msg.AddrList), NAToS(addr))
					addNodes(msg.AddrList)
					finished <- true
				}
			},
		},
	}

	// create peer.Peer instance
	p, err := peer.NewOutboundPeer(peerCfg, NToS(*node))
	if err != nil {
		//fmt.Printf("NewOutboundPeer: %v\n", err)
		//failed <- true
		missVisit(*node)
		return
	}

	// give peer.Peer instance a tcp connection
	d := net.Dialer{Timeout: time.Second}
	conn, err := d.Dial("tcp", p.Addr())
	if err != nil {
		//fmt.Printf("net.Dial: %v\n", err)
		//failed <- true
		missVisit(*node)
		return
	}
	p.AssociateConnection(conn)

	// wait for completion or timeout
	//fmt.Println("connected")
	select {
	case <-finished:
	//case <-failed:
	//missVisit(*node)
	case <-time.After(time.Second * 3):
		if !handshakeSuccessful {
			//fmt.Println("timeout")
			missVisit(*node)
		}
	}

	p.Disconnect()
	p.WaitForDisconnect()
	//fmt.Println("disconnected\n")
}

func worker() {
	for true {
		node := <-inbox
		visit(&node)
	}
}

func collect() {
	for true {
		online := onlineNodes()
		offline := offlineNodes()
		fmt.Printf("online %d, offline %d, inbox %d\n",
			len(online), len(offline), len(inbox))
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
