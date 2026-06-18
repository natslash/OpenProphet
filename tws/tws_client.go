// Package tws is a from-scratch Go client for the Interactive Brokers TWS API
// socket protocol. It talks to IB Gateway / TWS over TCP using the v100+
// length-prefixed, null-delimited message framing — no third-party library.
//
// It provides connection lifecycle management (handshake, startApi) and
// natively integrates the encoder and decoder components.
package tws

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// Client version range advertised in the handshake. The server negotiates down
// to min(serverMax, maxClientVer); bump maxClientVer when moving to a newer jar.
const (
	minClientVer = 100
	maxClientVer = 187
)

// Client is a single TWS socket connection.
type Client struct {
	host     string
	port     int
	clientID int

	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
	decoder *Decoder

	serverVersion int
	connTime      string

	mu        sync.RWMutex
	accounts  string
	orderIDs  OrderIdManager
	connected bool
	dispatcher *Dispatcher

	AsyncErrorCallback func(reqID, code int, msg string)
	appWrapper         Wrapper

	nextIDCh  chan int64 // first nextValidId is delivered here
	acctCh    chan string
	errCh     chan error // a fatal error seen during connect
	closed    chan struct{}
	closeOnce sync.Once
}

// NewClient builds a client. clientID must be unique across concurrent API
// connections to the same Gateway, or the Gateway rejects it (error 326).
func NewClient(host string, port, clientID int) *Client {
	c := &Client{
		host:     host,
		port:     port,
		clientID: clientID,
		nextIDCh: make(chan int64, 1),
		acctCh:     make(chan string, 1),
		errCh:      make(chan error, 1),
		closed:     make(chan struct{}),
		dispatcher: NewDispatcher(),
	}
	c.decoder = NewDecoder(c)
	return c
}

// Connect dials the Gateway, performs the handshake and startApi, starts the
// reader, and blocks until the first nextValidId arrives, a fatal error is
// reported, or ctx is done. Pass a ctx with a timeout.
func (c *Client) Connect(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			c.Close()
		}
	}()

	var d net.Dialer
	conn, dialErr := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", c.host, c.port))
	if dialErr != nil {
		return fmt.Errorf("dial %s:%d: %w", c.host, c.port, dialErr)
	}
	c.conn = conn
	c.reader = bufio.NewReader(conn)

	// Bound the synchronous handshake by the context deadline.
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	if err = c.handshake(); err != nil {
		return err
	}
	if err = c.startAPI(); err != nil {
		return err
	}

	// Clear the deadline so the reader can block indefinitely once connected.
	_ = conn.SetDeadline(time.Time{})
	go c.readLoop()

	var gotNextID, gotAccounts bool
	for !gotNextID || !gotAccounts {
		select {
		case <-c.nextIDCh:
			gotNextID = true
		case acct := <-c.acctCh:
			c.mu.Lock()
			c.accounts = acct
			c.mu.Unlock()
			gotAccounts = true
		case loopErr := <-c.errCh:
			return loopErr
		case <-ctx.Done():
			return fmt.Errorf("connected but missing initialization before deadline: %w", ctx.Err())
		case <-c.closed:
			return fmt.Errorf("connection closed before initialization finished")
		}
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()
	return nil
}

// handshake performs the v100+ greeting and reads the server version + time.
func (c *Client) handshake() error {
	greeting := fmt.Sprintf("v%d..%d", minClientVer, maxClientVer)
	if _, err := c.conn.Write([]byte("API\x00")); err != nil {
		return fmt.Errorf("write API prefix: %w", err)
	}
	if err := c.writeFrame([]byte(greeting)); err != nil {
		return fmt.Errorf("write greeting: %w", err)
	}
	payload, err := c.readFrame()
	if err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}
	f := splitFields(payload)
	if len(f) < 2 {
		return fmt.Errorf("malformed handshake: %q", string(payload))
	}
	sv, err := strconv.Atoi(f[0])
	if err != nil {
		return fmt.Errorf("bad server version %q: %w", f[0], err)
	}
	c.serverVersion = sv
	c.connTime = f[1]
	return nil
}

// startAPI sends the startApi message: msgId, version(2), clientId, optCaps.
func (c *Client) startAPI() error {
	return c.SendFields(strconv.Itoa(outStartAPI), "2", strconv.Itoa(c.clientID), "")
}

func (c *Client) readLoop() {
	for {
		payload, err := c.readFrame()
		if err != nil {
			c.signalClosed()
			return
		}
		fields := splitFields(payload)
		fmt.Printf("RECV: %q\n", fields)
		if err := c.decoder.Decode(fields); err != nil {
			fmt.Fprintf(os.Stderr, "tws_client: decode error: %v\n", err)
		}
	}
}

// Wrapper implementation for internal lifecycle routing.

func (c *Client) NextValidId(orderId int64) {
	c.orderIDs.Seed(orderId)
	select {
	case c.nextIDCh <- orderId:
	default: // first one already delivered
	}
	c.mu.RLock()
	appWrapper := c.appWrapper
	c.mu.RUnlock()
	if appWrapper != nil {
		appWrapper.NextValidId(orderId)
	}
}

// NextOrderId atomically acquires the next valid order ID from the client's manager.
func (c *Client) NextOrderId() int64 {
	return c.orderIDs.Next()
}

func (c *Client) ManagedAccounts(accountsList string) {
	select {
	case c.acctCh <- accountsList:
	default:
	}
	c.mu.Lock()
	c.accounts = accountsList
	c.mu.Unlock()
	
	c.mu.RLock()
	appWrapper := c.appWrapper
	c.mu.RUnlock()
	if appWrapper != nil {
		appWrapper.ManagedAccounts(accountsList)
	}
}

func (c *Client) Error(reqId int, code int, msg string) {
	c.mu.RLock()
	appWrapper := c.appWrapper
	c.mu.RUnlock()

	if isInfoCode(code) {
		if appWrapper != nil {
			appWrapper.Error(reqId, code, msg)
		}
		return
	}

	if reqId > 0 {
		c.dispatcher.Dispatch(int64(reqId), fmt.Errorf("TWS Error %d: %s", code, msg))
		c.dispatcher.Complete(int64(reqId))
	}

	c.mu.RLock()
	isConnected := c.connected
	cb := c.AsyncErrorCallback
	c.mu.RUnlock()

	if cb != nil {
		cb(reqId, code, msg)
	}
	if appWrapper != nil {
		appWrapper.Error(reqId, code, msg)
	}

	if !isConnected {
		select {
		case c.errCh <- fmt.Errorf("TWS error (id %d, code %d): %s", reqId, code, msg):
		default:
		}
	}
}

func (c *Client) CurrentTime(timeInSeconds int64) {
	c.mu.RLock()
	appWrapper := c.appWrapper
	c.mu.RUnlock()
	if appWrapper != nil {
		appWrapper.CurrentTime(timeInSeconds)
	}
}

// isInfoCode reports whether a TWS message code is informational rather than a
// real error (data-farm connection notices and connectivity warnings).
func isInfoCode(code int) bool {
	return (code >= 2100 && code <= 2200) || (code >= 1100 && code <= 1102)
}

// Accessors.

func (c *Client) ServerVersion() int    { return c.serverVersion }
func (c *Client) ConnectionTime() string { return c.connTime }



func (c *Client) Accounts() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accounts
}

func (c *Client) Close() error {
	c.signalClosed()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) signalClosed() {
	c.closeOnce.Do(func() { close(c.closed) })
}

// --- framing helpers ---

// sendFields writes one TWS message: each field null-terminated, the whole
// body length-prefixed with a 4-byte big-endian length.
func (c *Client) SendFields(fields ...string) error {
	fmt.Printf("SEND: %q\n", fields)
	body := []byte(strings.Join(fields, "\x00") + "\x00")
	return c.writeFrame(body)
}

func (c *Client) Encoder() *Encoder {
	return NewEncoder(c)
}

func (c *Client) Dispatcher() *Dispatcher {
	return c.dispatcher
}

func (c *Client) writeFrame(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(payload)))
	copy(buf[4:], payload)
	_, err := c.conn.Write(buf)
	return err
}

func (c *Client) readFrame() ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(c.reader, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > 1<<24 {
		return nil, fmt.Errorf("implausible frame length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(c.reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func splitFields(payload []byte) []string {
	if len(payload) == 0 {
		return nil
	}
	s := string(payload)
	if strings.HasSuffix(s, "\x00") {
		s = s[:len(s)-1]
	}
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\x00")
}

// ReqContractDetails fetches contract details synchronously using the dispatcher.
func (c *Client) ReqContractDetails(ctx context.Context, contract Contract) ([]ContractDetails, error) {
	reqId := c.NextOrderId()
	ch := c.dispatcher.Register(reqId)
	
	if err := c.Encoder().ReqContractDetails(reqId, contract); err != nil {
		c.dispatcher.Complete(reqId)
		return nil, err
	}
	
	var results []ContractDetails
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return results, nil
			}
			if cd, isCd := msg.(ContractDetails); isCd {
				results = append(results, cd)
			} else if errWrapper, isErr := msg.(error); isErr {
				return results, errWrapper
			}
		case <-ctx.Done():
			c.dispatcher.Complete(reqId)
			return nil, ctx.Err()
		}
	}
}

func (c *Client) ContractDetails(reqId int64, details ContractDetails) {
	c.dispatcher.Dispatch(reqId, details)
	c.mu.RLock()
	appWrapper := c.appWrapper
	c.mu.RUnlock()
	if appWrapper != nil {
		appWrapper.ContractDetails(reqId, details)
	}
}

func (c *Client) ContractDetailsEnd(reqId int64) {
	c.dispatcher.Complete(reqId)
	c.mu.RLock()
	appWrapper := c.appWrapper
	c.mu.RUnlock()
	if appWrapper != nil {
		appWrapper.ContractDetailsEnd(reqId)
	}
}

func (c *Client) TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr TickAttrib) {
	c.dispatcher.Dispatch(reqId, TickPriceMsg{TickType: tickType, Price: price, Size: size, Attr: attr})
	c.mu.RLock()
	appWrapper := c.appWrapper
	c.mu.RUnlock()
	if appWrapper != nil {
		appWrapper.TickPrice(reqId, tickType, price, size, attr)
	}
}

func (c *Client) TickSize(reqId int64, tickType int, size decimal.Decimal) {
	c.dispatcher.Dispatch(reqId, TickSizeMsg{TickType: tickType, Size: size})
	c.mu.RLock()
	appWrapper := c.appWrapper
	c.mu.RUnlock()
	if appWrapper != nil {
		appWrapper.TickSize(reqId, tickType, size)
	}
}

func (c *Client) HistoricalData(reqId int64, data HistoricalData) {
	c.dispatcher.Dispatch(reqId, data)
	c.mu.RLock()
	appWrapper := c.appWrapper
	c.mu.RUnlock()
	if appWrapper != nil {
		appWrapper.HistoricalData(reqId, data)
	}
}
