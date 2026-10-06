package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type item struct {
	a []string
	t time.Time
}

func name(b []byte, p int) (string, int) {
	a := []string{}
	e := p
	j := false
	for {
		n := int(b[p])
		if n&0xc0 == 0xc0 {
			if !j {
				e = p + 2
				j = true
			}
			p = int(binary.BigEndian.Uint16(b[p:p+2]) & 0x3fff)
			continue
		}
		p++
		if n == 0 {
			if !j {
				e = p
			}
			break
		}
		a = append(a, string(b[p:p+n]))
		p += n
	}
	return strings.Join(a, ".") + ".", e
}

func parse(b []byte) (string, []string, uint32, int) {
	rc := int(binary.BigEndian.Uint16(b[2:4]) & 15)
	q := int(binary.BigEndian.Uint16(b[4:6]))
	n := int(binary.BigEndian.Uint16(b[6:8]))
	st := map[int]string{
		0: "NOERROR",
		1: "FORMERR",
		2: "SERVFAIL",
		3: "NXDOMAIN",
		5: "REFUSED",
	}
	s := st[rc]
	if s == "" {
		s = fmt.Sprintf("RCODE%d", rc)
	}
	p := 12
	for i := 0; i < q; i++ {
		_, p = name(b, p)
		p += 4
	}
	a := []string{}
	var mn uint32
	for i := 0; i < n; i++ {
		_, p = name(b, p)
		t := binary.BigEndian.Uint16(b[p : p+2])
		ttl := binary.BigEndian.Uint32(b[p+4 : p+8])
		l := int(binary.BigEndian.Uint16(b[p+8 : p+10]))
		p += 10
		x := p
		if i == 0 || ttl < mn {
			mn = ttl
		}
		if t == 1 {
			a = append(a, fmt.Sprintf("A %s %d", net.IP(b[x:x+4]), ttl))
		}
		if t == 28 {
			a = append(a, fmt.Sprintf("AAAA %s %d", net.IP(b[x:x+16]), ttl))
		}

		if t == 5 || t == 2 {
			v, _ := name(b, x)
			k := "CNAME"

			if t == 2 {
				k = "NS"
			}

			a = append(a, fmt.Sprintf("%s %s %d", k, v, ttl))
		}

		if t == 15 {
			pr := binary.BigEndian.Uint16(b[x : x+2])
			v, _ := name(b, x+2)
			a = append(a, fmt.Sprintf("MX %d %s %d", pr, v, ttl))
		}

		if t == 16 {
			v := ""
			z := b[x : x+l]

			for len(z) > 0 {
				k := int(z[0])
				z = z[1:]
				v += string(z[:k])
				z = z[k:]
			}

			a = append(a, fmt.Sprintf("TXT %s %d", v, ttl))
		}

		p = x + l
	}

	return s, a, mn, n
}

func main() {
	if len(os.Args) < 3 {
		os.Exit(1)
	}

	types := map[string]uint16{
		"A":     1,
		"NS":    2,
		"CNAME": 5,
		"MX":    15,
		"TXT":   16,
		"AAAA":  28,
	}

	cache := map[string]item{}
	sc := bufio.NewScanner(os.Stdin)

	var id uint16

	for sc.Scan() {
		x := strings.Fields(sc.Text())

		if len(x) == 0 {
			continue
		}

		nm := x[0]
		tp := x[1]
		key := strings.ToLower(nm) + " " + tp

		fmt.Println("query", nm, tp)

		if v, ok := cache[key]; ok && time.Now().Before(v.t) {
			fmt.Println("status NOERROR")

			for _, s := range v.a {
				fmt.Println("answer", s)
			}

			fmt.Println("end")
			continue
		}

		id++

		q := make([]byte, 12)

		binary.BigEndian.PutUint16(q[:2], id)
		binary.BigEndian.PutUint16(q[2:4], 0x0100)
		binary.BigEndian.PutUint16(q[4:6], 1)

		for _, s := range strings.Split(strings.TrimSuffix(nm, "."), ".") {
			q = append(q, byte(len(s)))
			q = append(q, s...)
		}

		q = append(q, 0)

		z := make([]byte, 4)

		binary.BigEndian.PutUint16(z[:2], types[tp])
		binary.BigEndian.PutUint16(z[2:], 1)

		q = append(q, z...)

		addr, e := net.ResolveUDPAddr("udp", net.JoinHostPort(os.Args[1], os.Args[2]))

		if e != nil {
			os.Exit(1)
		}

		c, e := net.DialUDP("udp", nil, addr)

		if e != nil {
			os.Exit(1)
		}

		c.SetDeadline(time.Now().Add(5 * time.Second))
		c.Write(q)

		b := make([]byte, 4096)
		var r []byte

		for {
			n, _, e := c.ReadFromUDP(b)

			if e != nil {
				c.Close()
				fmt.Println("status TIMEOUT")
				fmt.Println("end")
				os.Exit(1)
			}

			if binary.BigEndian.Uint16(b[:2]) == id {
				r = b[:n]
				break
			}
		}

		c.Close()

		st, a, ttl, n := parse(r)

		fmt.Println("status", st)

		for _, s := range a {
			fmt.Println("answer", s)
		}

		fmt.Println("end")

		if st == "NOERROR" && n > 0 && ttl > 0 {
			cache[key] = item{
				a: a,
				t: time.Now().Add(time.Duration(ttl) * time.Second),
			}
		}
	}
}
