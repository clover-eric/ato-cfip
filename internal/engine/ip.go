package engine

import (
	"bufio"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
)

type ipRanges struct {
	ips     []*net.IPAddr
	mask    string
	firstIP net.IP
	ipNet   *net.IPNet
	allIP   bool
}

func loadIPRanges(file, inline string, allowIPv6, allIP bool) ([]*net.IPAddr, error) {
	r := &ipRanges{ips: make([]*net.IPAddr, 0), allIP: allIP}
	var lines []string
	if inline != "" {
		lines = strings.Split(inline, ",")
	} else {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !allowIPv6 && !isIPv4(line) {
			continue
		}
		if err := r.parseCIDR(line); err != nil {
			return nil, err
		}
		if isIPv4(line) {
			r.chooseIPv4()
		} else {
			r.chooseIPv6()
		}
	}
	return r.ips, nil
}

func isIPv4(ip string) bool {
	return strings.Contains(ip, ".")
}

func randIPEndWith(num byte) byte {
	if num == 0 {
		return 0
	}
	return byte(rand.Intn(int(num)))
}

func (r *ipRanges) fixIP(ip string) string {
	if i := strings.IndexByte(ip, '/'); i < 0 {
		if isIPv4(ip) {
			r.mask = "/32"
		} else {
			r.mask = "/128"
		}
		return ip + r.mask
	}
	r.mask = ip[strings.IndexByte(ip, '/'):]
	return ip
}

func (r *ipRanges) parseCIDR(ip string) error {
	firstIP, ipNet, err := net.ParseCIDR(r.fixIP(ip))
	if err != nil {
		return fmt.Errorf("parse CIDR %q: %w", ip, err)
	}
	r.firstIP = firstIP
	r.ipNet = ipNet
	return nil
}

func (r *ipRanges) appendIPv4(d byte) {
	r.appendIP(net.IPv4(r.firstIP[12], r.firstIP[13], r.firstIP[14], d))
}

func (r *ipRanges) appendIP(ip net.IP) {
	target := make(net.IP, len(ip))
	copy(target, ip)
	r.ips = append(r.ips, &net.IPAddr{IP: target})
}

func (r *ipRanges) getIPRange() (minIP, hosts byte) {
	minIP = r.firstIP[15] & r.ipNet.Mask[3]
	m := net.IPv4Mask(255, 255, 255, 255)
	for i, v := range r.ipNet.Mask {
		m[i] ^= v
	}
	total, _ := strconv.ParseInt(m.String(), 16, 32)
	if total > 255 {
		return minIP, 255
	}
	return minIP, byte(total)
}

func (r *ipRanges) chooseIPv4() {
	if r.mask == "/32" {
		r.appendIP(r.firstIP)
		return
	}
	minIP, hosts := r.getIPRange()
	for r.ipNet.Contains(r.firstIP) {
		if r.allIP {
			for i := 0; i <= int(hosts); i++ {
				r.appendIPv4(byte(i) + minIP)
			}
		} else {
			r.appendIPv4(minIP + randIPEndWith(hosts))
		}
		r.firstIP[14]++
		if r.firstIP[14] == 0 {
			r.firstIP[13]++
			if r.firstIP[13] == 0 {
				r.firstIP[12]++
			}
		}
	}
}

func (r *ipRanges) chooseIPv6() {
	if r.mask == "/128" {
		r.appendIP(r.firstIP)
		return
	}
	var tempIP uint8
	for r.ipNet.Contains(r.firstIP) {
		r.firstIP[15] = randIPEndWith(255)
		r.firstIP[14] = randIPEndWith(255)
		r.appendIP(r.firstIP)
		for i := 13; i >= 0; i-- {
			tempIP = r.firstIP[i]
			r.firstIP[i] += randIPEndWith(255)
			if r.firstIP[i] >= tempIP {
				break
			}
		}
	}
}
