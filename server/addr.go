package main

// this code has been taken from: https://gist.github.com/andres-erbsen/62d7defe8dce2e182bd9a90da2e1f659#file-addr-go

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

func rfc1918private(ip net.IP) (bool, error) {
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		_, subnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return false, fmt.Errorf("failed to parse hardcoded rfc1918 cidr: %v", err)
		}
		if subnet.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
}

func rfc4193private(ip net.IP) (bool, error) {
	_, subnet, err := net.ParseCIDR("fd00::/8")
	if err != nil {
		return false, fmt.Errorf("failed to parse hardcoded rfc4193 cidr: %v", err)
	}
	return subnet.Contains(ip), nil
}

func isLoopback(ip net.IP) (bool, error) {
	for _, cidr := range []string{"127.0.0.0/8", "::1/128"} {
		_, subnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return false, fmt.Errorf("failed to parse hardcoded loopback cidr: %v", err)
		}
		if subnet.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
}

func possiblePublicIP(ip net.IP) (t bool, err error) {
	var rfc1918, rfc4193, isLoop bool
	if rfc1918, err = rfc1918private(ip); err != nil {
		return false, fmt.Errorf("possible: %v", err)
	}
	if rfc4193, err = rfc4193private(ip); err != nil {
		return false, fmt.Errorf("possible: %v", err)
	}
	if isLoop, err = isLoopback(ip); err != nil {
		return false, fmt.Errorf("possible: %v", err)
	}
	return !rfc1918 && !rfc4193 && !isLoop, nil
}

type ipName struct {
	name string
	ip   net.IP
}

func heuristic(ni ipName) (ret int) {
	var ok bool
	n, i := strings.ToLower(ni.name), ni.ip
	if ok, _ = isLoopback(i); ok {
		ret += 1000
	}
	if ok, _ = rfc1918private(i); ok {
		ret += 500
	} else if ok, _ = rfc4193private(i); ok {
		ret += 500
	}
	if strings.Contains(n, "dyn") {
		ret += 100
	}
	if strings.Contains(n, "dhcp") {
		ret += 99
	}
	for j := 0; j < len(i); j++ {
		if strings.Contains(n, strconv.Itoa(int(i[j]))) {
			ret += 5
		}
	}
	return ret
}

type byHeuristic []ipName

func (nis byHeuristic) Len() int      { return len(nis) }
func (nis byHeuristic) Swap(i, j int) { nis[i], nis[j] = nis[j], nis[i] }
func (nis byHeuristic) Less(i, j int) bool {
	return heuristic(nis[i]) < heuristic(nis[j])
}

func publicAddresses() ([]ipName, error) {
	var ret []ipName

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			return nil, err
		}
		// ignore unresolvable addresses
		names, err := net.LookupAddr(ip.String())
		if err != nil {
			continue
		}
		for _, name := range names {
			ret = append(ret, ipName{name, ip})
		}
	}

	sort.Sort(byHeuristic(ret))
	return ret, nil
}
