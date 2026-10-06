package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func subnet(s string) {
	_, n, e := net.ParseCIDR(s)
	if e != nil {
		os.Exit(1)
	}

	p, _ := n.Mask.Size()
	a := n.IP.To4()
	m := net.IP(n.Mask).To4()

	b := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		b[i] = a[i] | ^m[i]
	}

	fmt.Println("network", a)
	fmt.Println("netmask", m)
	fmt.Println("prefix", p)

	if p == 32 {
		fmt.Println("broadcast", "none")
		fmt.Println("first", a)
		fmt.Println("last", a)
		fmt.Println("hosts", 1)
		return
	}

	if p == 31 {
		fmt.Println("broadcast", "none")
		fmt.Println("first", a)
		fmt.Println("last", b)
		fmt.Println("hosts", 2)
		return
	}

	f := append(net.IP{}, a...)
	l := append(net.IP{}, b...)

	f[3]++
	l[3]--

	fmt.Println("broadcast", b)
	fmt.Println("first", f)
	fmt.Println("last", l)
	fmt.Println("hosts", (uint64(1)<<(32-p))-2)
}

func route(file, dst string) {
	f, e := os.Open(file)
	if e != nil {
		os.Exit(1)
	}
	defer f.Close()

	d := net.ParseIP(dst)
	best := -1
	via := ""

	sc := bufio.NewScanner(f)

	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())

		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}

		x := strings.Fields(s)

		_, n, e := net.ParseCIDR(x[0])
		if e != nil {
			continue
		}

		p, _ := n.Mask.Size()

		if n.Contains(d) && p > best {
			best = p
			via = x[1]
		}
	}

	if best == -1 {
		fmt.Println("unreachable", true)
		os.Exit(1)
	}

	fmt.Println("via", via)
	fmt.Println("prefix", best)
}

func main() {
	if len(os.Args) < 3 {
		os.Exit(1)
	}

	if os.Args[1] == "subnet" {
		subnet(os.Args[2])
	} else if os.Args[1] == "route" && len(os.Args) >= 4 {
		route(os.Args[2], os.Args[3])
	}
}
