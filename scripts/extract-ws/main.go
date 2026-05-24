package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// This tool extracts WebSocket payloads and DNS queries from a .pcap file
// and prints them in a format compatible with soundtouch-service interactions.

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run scripts/extract-ws.go <pcap_file> [filter_ip]")
		os.Exit(1)
	}

	pcapFile := os.Args[1]
	filterIP := ""
	if len(os.Args) > 2 {
		filterIP = os.Args[2]
		fmt.Printf("[DEBUG] Filtering WebSocket for IP: %s\n", filterIP)
	}

	handle, err := pcap.OpenOffline(pcapFile)
	if err != nil {
		log.Fatal(err)
	}
	defer handle.Close()

	fmt.Printf("[DEBUG] Reading file: %s\n", pcapFile)

	// Prepare output files
	baseName := strings.TrimSuffix(pcapFile, filepath.Ext(pcapFile))
	wsFile, err := os.Create(baseName + ".ws.http")
	if err != nil {
		log.Fatal(err)
	}
	defer wsFile.Close()

	dnsFile, err := os.Create(baseName + ".dns.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer dnsFile.Close()

	mdnsFile, err := os.Create(baseName + ".mdns.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer mdnsFile.Close()

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	for packet := range packetSource.Packets() {
		// Handle DNS
		if dnsLayer := packet.Layer(layers.LayerTypeDNS); dnsLayer != nil {
			dns, _ := dnsLayer.(*layers.DNS)
			extractDNS(packet, dns, dnsFile, mdnsFile)
		}

		// Handle SSDP (UDP Port 1900)
		if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
			udp, _ := udpLayer.(*layers.UDP)
			if udp.DstPort == 1900 || udp.SrcPort == 1900 {
				extractSSDP(packet, udp, baseName+".ssdp.txt")
			}
		}

		// Handle WebSockets (TCP)
		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			tcp, _ := tcpLayer.(*layers.TCP)
			extractWebSocket(packet, tcp, filterIP, wsFile)
		}
	}

	fmt.Printf("[DEBUG] Extraction complete. Results written to:\n- %s\n- %s\n- %s\n- %s\n",
		baseName+".ws.http", baseName+".dns.txt", baseName+".mdns.txt", baseName+".ssdp.txt")
}

func extractSSDP(packet gopacket.Packet, udp *layers.UDP, ssdpFilename string) {
	payload := string(udp.Payload)
	if !strings.Contains(payload, "HTTP/1.1") && !strings.Contains(payload, "NOTIFY") && !strings.Contains(payload, "M-SEARCH") {
		return
	}

	f, err := os.OpenFile(ssdpFilename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	srcIP := packet.NetworkLayer().NetworkFlow().Src().String()
	dstIP := packet.NetworkLayer().NetworkFlow().Dst().String()
	timestamp := packet.Metadata().Timestamp.Format("2006-01-02 15:04:05.000")

	fmt.Fprintf(f, "[%s] %s:%d -> %s:%d\n", timestamp, srcIP, udp.SrcPort, dstIP, udp.DstPort)
	fmt.Fprintf(f, "%s\n", strings.TrimSpace(payload))
	fmt.Fprintln(f, "-------------------------------------------------")
}

func extractDNS(packet gopacket.Packet, dns *layers.DNS, dnsFile, mdnsFile *os.File) {
	srcIP := packet.NetworkLayer().NetworkFlow().Src().String()
	dstIP := packet.NetworkLayer().NetworkFlow().Dst().String()

	isMDNS := false
	if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
		udp, _ := udpLayer.(*layers.UDP)
		if udp.DstPort == 5353 || udp.SrcPort == 5353 {
			isMDNS = true
		}
	}

	out := dnsFile
	if isMDNS {
		out = mdnsFile
	}

	timestamp := packet.Metadata().Timestamp.Format("2006-01-02 15:04:05.000")
	prefix := fmt.Sprintf("[%s] %s -> %s", timestamp, srcIP, dstIP)

	for _, q := range dns.Questions {
		fmt.Fprintf(out, "%s | QUERY: %s (%s)\n", prefix, string(q.Name), q.Type)
	}
	for _, a := range dns.Answers {
		val := ""
		if a.IP != nil {
			val = a.IP.String()
		} else if len(a.CNAME) > 0 {
			val = string(a.CNAME)
		} else if len(a.PTR) > 0 {
			val = string(a.PTR)
		} else if len(a.TXTs) > 0 {
			var txts []string
			for _, t := range a.TXTs {
				txts = append(txts, string(t))
			}
			val = strings.Join(txts, " ")
		} else {
			val = fmt.Sprintf("Type: %s", a.Type)
		}
		fmt.Fprintf(out, "%s | ANSWER: %s -> %s\n", prefix, string(a.Name), val)
	}
}

func extractWebSocket(packet gopacket.Packet, tcp *layers.TCP, filterIP string, wsFile *os.File) {
	srcIP := packet.NetworkLayer().NetworkFlow().Src().String()
	dstIP := packet.NetworkLayer().NetworkFlow().Dst().String()

	if filterIP != "" && srcIP != filterIP && dstIP != filterIP {
		return
	}

	payload := tcp.Payload
	if len(payload) == 0 {
		return
	}

	// Check for WebSocket Frame (Sliding search)
	for i := 0; i < len(payload)-2; i++ {
		firstByte := payload[i]
		// Opcode 1 (Text) or 2 (Binary).
		if (firstByte&0xF0) == 0x80 && (firstByte&0x0F == 1 || firstByte&0x0F == 2) {
			secondByte := payload[i+1]
			mask := (secondByte & 0x80) != 0
			length := int(secondByte & 0x7F)
			offset := i + 2

			if length == 126 {
				if len(payload) < offset+2 {
					continue
				}
				length = int(payload[offset])<<8 | int(payload[offset+1])
				offset += 2
			} else if length == 127 {
				if len(payload) < offset+8 {
					continue
				}
				length = int(payload[offset+4])<<24 | int(payload[offset+5])<<16 | int(payload[offset+6])<<8 | int(payload[offset+7])
				offset += 8
			}

			if mask {
				if len(payload) < offset+4+length {
					continue
				}
				maskKey := payload[offset : offset+4]
				offset += 4
				data := make([]byte, length)
				for j := 0; j < length; j++ {
					data[j] = payload[offset+j] ^ maskKey[j%4]
				}
				printInteraction(packet, tcp, data, wsFile)
				i = offset + length - 1
			} else {
				if len(payload) >= offset+length {
					data := payload[offset : offset+length]
					printInteraction(packet, tcp, data, wsFile)
					i = offset + length - 1
				}
			}
		}
	}
}

func printInteraction(packet gopacket.Packet, tcp *layers.TCP, data []byte, out io.Writer) {
	src := packet.NetworkLayer().NetworkFlow().Src().String()
	dst := packet.NetworkLayer().NetworkFlow().Dst().String()

	fmt.Fprintf(out, "### WebSocket Message: %s -> %s\n", src, dst)
	fmt.Fprintf(out, "// Timestamp: %s\n", packet.Metadata().Timestamp)
	fmt.Fprintf(out, "// Ports: %d -> %d\n", tcp.SrcPort, tcp.DstPort)
	fmt.Fprintln(out)

	// Try to detect if it's GZIP
	content := ""
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		fmt.Fprintln(out, "// [Detected GZIP compression]")
		content = decompressGzip(data)
	} else {
		content = string(data)
	}

	fmt.Fprintln(out, "/*")
	fmt.Fprintln(out, strings.TrimSpace(content))
	fmt.Fprintln(out, "*/")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "-------------------------------------------------")
	fmt.Fprintln(out, "")
}

func decompressGzip(data []byte) string {
	b := bytes.NewBuffer(data)
	r, err := gzip.NewReader(b)
	if err != nil {
		return "[Error: Failed to create GZIP reader: " + err.Error() + "]"
	}
	defer r.Close()

	res, err := io.ReadAll(r)
	if err != nil {
		return "[Error: Failed to decompress GZIP: " + err.Error() + "]"
	}
	return string(res)
}
