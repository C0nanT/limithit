package metrics

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
)

// IPPool yields IP addresses for spoofing X-Forwarded-For. Thread-safe.
type IPPool struct {
	ips     []string
	idx     atomic.Uint64
	shuffle bool
}

func NewIPPoolFromList(ips []string) *IPPool {
	cp := make([]string, len(ips))
	copy(cp, ips)
	return &IPPool{ips: cp}
}

// NewIPPoolFromSpec accepts:
//   - "10.0.0.0/24"  (IPv4 CIDR, expanded — capped at 65536)
//   - "file:/path/to/ips.txt" (one IP per line)
//   - "1.2.3.4,5.6.7.8" (comma-separated)
func NewIPPoolFromSpec(spec string) (*IPPool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty ip-pool spec")
	}
	if strings.HasPrefix(spec, "file:") {
		return loadIPFile(strings.TrimPrefix(spec, "file:"))
	}
	if strings.Contains(spec, "/") {
		return expandCIDR(spec)
	}
	parts := strings.Split(spec, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := netip.ParseAddr(p); err != nil {
			return nil, fmt.Errorf("invalid ip %q: %w", p, err)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no ips parsed from spec")
	}
	return NewIPPoolFromList(out), nil
}

func loadIPFile(path string) (*IPPool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var ips []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := netip.ParseAddr(line); err != nil {
			return nil, fmt.Errorf("invalid ip %q: %w", line, err)
		}
		ips = append(ips, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("file %s contained no IPs", path)
	}
	return NewIPPoolFromList(ips), nil
}

const maxCIDRExpansion = 1 << 16

func expandCIDR(cidr string) (*IPPool, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	ip4 := ipnet.IP.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("only IPv4 CIDRs supported")
	}
	ones, bits := ipnet.Mask.Size()
	hostBits := bits - ones
	if hostBits < 0 {
		return nil, fmt.Errorf("invalid mask")
	}
	count := uint64(1) << uint(hostBits)
	if count > maxCIDRExpansion {
		count = maxCIDRExpansion
	}
	base := binary.BigEndian.Uint32(ip4)
	ips := make([]string, 0, count)
	for i := uint64(0); i < count; i++ {
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], base+uint32(i))
		ips = append(ips, net.IP(buf[:]).String())
	}
	pool := NewIPPoolFromList(ips)
	pool.shuffle = true
	rand.Shuffle(len(pool.ips), func(i, j int) { pool.ips[i], pool.ips[j] = pool.ips[j], pool.ips[i] })
	return pool, nil
}

func (p *IPPool) Next() string {
	if len(p.ips) == 0 {
		return ""
	}
	n := p.idx.Add(1) - 1
	return p.ips[int(n%uint64(len(p.ips)))]
}

func (p *IPPool) Size() int { return len(p.ips) }
