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

// globals
var lock = sync.RWMutex{}
var inbox = make(chan Node, 10000000)
var nodes = make(map[string]Node)

// Node represents a bitcoin peer
type Node struct {
	IP              string
	Port            int
	VisitsMissed    int
	LastVisitMissed int
}

// update single record in `nodes` map
func writeNode(node Node) {
	lock.Lock()
	defer lock.Unlock()
	nodes[node.IP] = node
}

// read single record in `nodes` map
func readNode(ip string) (Node, bool) {
	lock.RLock()
	defer lock.RUnlock()
	node, ok := nodes[ip]
	return node, ok
}

// add a list of wire.NetAddress's to `nodes` map
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

// look up entries in `nodes` which haven't missed a visit
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

// look up entries in `nodes` which have missed a visit
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

// look up entries in `nodes` which we haven't yet attempted to visit
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

// increment `visitsMissed` on a `nodes` record
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

// rese `visitsMissed` to 0 for a `nodes` record
func makeVisit(node Node) {
	node.VisitsMissed = 0
	writeNode(node)
}

// HandleVerack prints when we've received a verack message
// load some wire.NetAddress instances into the inbox channel
func queryDNS() {
	params := chaincfg.MainNetParams
	defaultRequiredServices := wire.SFNodeNetwork
	connmgr.SeedFromDNS(&params, defaultRequiredServices,
		net.LookupIP, addNodes)
	<-time.After(time.Second * 10) // HACK!!!
}

// NToS converts a node to a string
func NToS(node Node) string {
	return fmt.Sprintf("[%s]:%d", node.IP, node.Port)
}

// visit a node
// update record in `nodes` based on whether version handshake succeeded
// send `getaddr` and wait for `addr` response, adding new entries to
// `nodes` if we receive one
func visit(node *Node) {
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
				makeVisit(*node)
				handshakeSuccessful = true
			},
			OnAddr: func(p *peer.Peer, msg *wire.MsgAddr) {
				if len(msg.AddrList) > 1 {
					addNodes(msg.AddrList)
					finished <- true
				}
			},
		},
	}

	// create peer.Peer instance
	p, err := peer.NewOutboundPeer(peerCfg, NToS(*node))
	if err != nil {
		missVisit(*node)
		return
	}

	// give peer.Peer instance a tcp connection
	d := net.Dialer{Timeout: time.Second}
	conn, err := d.Dial("tcp", p.Addr())
	if err != nil {
		missVisit(*node)
		return
	}
	p.AssociateConnection(conn)

	// wait for completion or timeout
	select {
	case <-finished:
	case <-time.After(time.Second * 3):
		if !handshakeSuccessful {
			missVisit(*node)
		}
	}

	p.Disconnect()
	p.WaitForDisconnect()
}

// loops forever connecting to nodes waiting in `inbox` channel
func worker() {
	for true {
		node := <-inbox
		visit(&node)
	}
}

// prints some stats once per second
func stats() {
	for true {
		online := onlineNodes()
		offline := offlineNodes()
		fmt.Printf("online %d, offline %d, inbox %d\n",
			len(online), len(offline), len(inbox))
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
	stats()
}
