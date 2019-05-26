package main

import (
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/wire"
	"net"
	"time"
)

func queryDNS() []*wire.NetAddress {
	allAddrs := make([]*wire.NetAddress, 10)
	params := chaincfg.MainNetParams
	defaultRequiredServices := wire.SFNodeNetwork
	connmgr.SeedFromDNS(&params, defaultRequiredServices,
		net.LookupIP, func(addrs []*wire.NetAddress) {
			//s.addrManager.AddAddresses(addrs, addrs[0])
			for _, addr := range addrs {
				allAddrs = append(allAddrs, addr)
			}
		})
	<-time.After(time.Second * 10)
	return allAddrs
}

func main() {
	fmt.Println(queryDNS())
}
