// Command twscheck is the Phase 0.3 TWS socket sanity check.
//
// It opens a TCP connection to IB Gateway / TWS, performs the v100+ handshake,
// and prints the negotiated server version and connection time.
//
// It deliberately does NOT call startApi and requests no market or account
// data, so it cannot start an API session or affect your account in any way.
// Its only job is to prove that (a) the socket is reachable and (b) our
// understanding of the wire framing is correct.
//
// Usage:
//
//	go run ./cmd/twscheck                 # paper IB Gateway, 127.0.0.1:4002 (default)
//	go run ./cmd/twscheck -port 7497      # paper TWS desktop
//	go run ./cmd/twscheck -host 10.0.0.5  # Gateway on another machine
//
// Ports: 4002 paper Gateway · 4001 live Gateway · 7497 paper TWS · 7496 live TWS.
package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

// Client version range we advertise in the handshake. The server negotiates
// down to min(serverMax, maxClientVer); bump maxClientVer when you move to a
// newer API jar to unlock newer fields.
const (
	minClientVer = 100
	maxClientVer = 187
)

func main() {
	host := flag.String("host", "127.0.0.1", "IB Gateway / TWS host")
	port := flag.Int("port", 4002, "API port (4002 paper Gateway, 4001 live Gateway, 7497 paper TWS, 7496 live TWS)")
	timeout := flag.Duration("timeout", 5*time.Second, "connect + read timeout")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("-> connecting to %s ...\n", addr)

	conn, err := net.DialTimeout("tcp", addr, *timeout)
	if err != nil {
		fail("connect failed: %v\n   Is IB Gateway running and is %d the configured API port?\n   Check: API > Settings > Enable ActiveX and Socket Clients, and add 127.0.0.1 as a Trusted IP.", err, *port)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(*timeout))

	// v100+ handshake:
	//   1. raw bytes        "API\0"
	//   2. length-prefixed  "v{MIN}..{MAX}"
	greeting := fmt.Sprintf("v%d..%d", minClientVer, maxClientVer)
	if _, err := conn.Write([]byte("API\x00")); err != nil {
		fail("writing API prefix: %v", err)
	}
	if err := writeFrame(conn, []byte(greeting)); err != nil {
		fail("writing greeting: %v", err)
	}

	// The server answers with a single framed message containing two
	// null-terminated fields: serverVersion and connectionTime.
	payload, err := readFrame(bufio.NewReader(conn))
	if err != nil {
		fail("no handshake response: %v\n   The socket opened but the Gateway did not reply. Usually 'Enable ActiveX and Socket Clients' is off, or 127.0.0.1 is not a Trusted IP.", err)
	}

	fields := strings.Split(strings.TrimRight(string(payload), "\x00"), "\x00")
	if len(fields) < 2 {
		fail("malformed handshake payload: %q", string(payload))
	}

	fmt.Println("OK  handshake succeeded")
	fmt.Printf("    server version : %s\n", fields[0])
	fmt.Printf("    connection time: %s\n", fields[1])
	fmt.Println()
	fmt.Println("No API session was started and no data was requested.")
	fmt.Println("This only proves the socket and protocol framing work.")
}

// writeFrame writes a 4-byte big-endian length prefix followed by payload.
func writeFrame(w io.Writer, payload []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// readFrame reads a 4-byte big-endian length prefix, then that many bytes.
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > 1<<20 {
		return nil, fmt.Errorf("implausible frame length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL  "+format+"\n", args...)
	os.Exit(1)
}
