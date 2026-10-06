package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

func ip(d []byte) {
	h := int(d[0]&15) * 4
	n := int(binary.BigEndian.Uint16(d[2:4]))
	x := binary.BigEndian.Uint16(d[6:8])
	p := d[9]
	f := ""
	if x&0x4000 != 0 {
		f = "DF"
	}
	if x&0x2000 != 0 {
		if f != "" {
			f += ","
		}
		f += "MF"
	}
	if f == "" {
		f = "none"
	}
	fmt.Println("ip.version", d[0]>>4)
	fmt.Println("ip.ihl_bytes", h)
	fmt.Println("ip.total_length", n)
	fmt.Println("ip.id", fmt.Sprintf("0x%04x", binary.BigEndian.Uint16(d[4:6])))
	fmt.Println("ip.flags", f)
	fmt.Println("ip.frag_offset", (x&0x1fff)*8)
	fmt.Println("ip.ttl", d[8])
	fmt.Println("ip.protocol", p)
	fmt.Println("ip.src", net.IP(d[12:16]))
	fmt.Println("ip.dst", net.IP(d[16:20]))
	fmt.Println("ip.checksum_valid", sum(d[:h]))
	if p == 6 {
		tcp(d[h:], n-h)
	} else if p == 17 {
		udp(d[h:], n-h)
	} else {
		fmt.Println("payload.length", n-h)
	}
}

func tcp(d []byte, n int) {
	h := int(d[12]>>4) * 4
	a := []string{"FIN", "SYN", "RST", "PSH", "ACK", "URG"}
	f := []string{}
	for i := 0; i < 6; i++ {
		if d[13]&(1<<i) != 0 {
			f = append(f, a[i])
		}
	}
	if len(f) == 0 {
		f = append(f, "none")
	}
	fmt.Println("tcp.src_port", binary.BigEndian.Uint16(d[:2]))
	fmt.Println("tcp.dst_port", binary.BigEndian.Uint16(d[2:4]))
	fmt.Println("tcp.seq", binary.BigEndian.Uint32(d[4:8]))
	fmt.Println("tcp.ack", binary.BigEndian.Uint32(d[8:12]))
	fmt.Println("tcp.data_offset_bytes", h)
	fmt.Println("tcp.flags", strings.Join(f, ","))
	fmt.Println("tcp.window", binary.BigEndian.Uint16(d[14:16]))
	fmt.Println("payload.length", n-h)
}

func udp(d []byte, n int) {
	fmt.Println("udp.src_port", binary.BigEndian.Uint16(d[:2]))
	fmt.Println("udp.dst_port", binary.BigEndian.Uint16(d[2:4]))
	fmt.Println("udp.length", binary.BigEndian.Uint16(d[4:6]))
	fmt.Println("payload.length", n-8)
}

func sum(d []byte) bool {
	var s uint32
	for i := 0; i < len(d); i += 2 {
		s += uint32(binary.BigEndian.Uint16(d[i : i+2]))
	}
	for s > 0xffff {
		s = s&0xffff + s>>16
	}
	return s == 0xffff
}

func main() {
	b, _ := io.ReadAll(os.Stdin)
	s := string(b)
	s = strings.Join(strings.Fields(s), "")
	s = strings.ReplaceAll(s, ":", "")
	d, e := hex.DecodeString(s)
	if e != nil || len(d) < 14 {
		os.Exit(1)
	}
	fmt.Println("eth.dst", net.HardwareAddr(d[:6]))
	fmt.Println("eth.src", net.HardwareAddr(d[6:12]))
	t := binary.BigEndian.Uint16(d[12:14])
	fmt.Println("eth.ethertype", fmt.Sprintf("0x%04x", t))
	if t == 0x0800 {
		ip(d[14:])
	}
}
