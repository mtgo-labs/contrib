//go:build !windows

package netpoll

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	cloudnetpoll "github.com/cloudwego/netpoll"
)

const defaultMaxPayload = 16 << 20

const (
	transportIntermediate uint8 = iota
	transportAbridged
	transportPaddedIntermediate
)

var (
	errInvalidPacket         = errors.New("netpoll: invalid packet")
	errInvalidPacketLength   = errors.New("netpoll: invalid packet length")
	errPacketLimit           = errors.New("netpoll: packet exceeds limit")
	errPacketNotConfigured   = errors.New("netpoll: packet transport is not configured")
	errPacketTransportInUse  = errors.New("netpoll: connection is already in use")
	errPacketReconfiguration = errors.New("netpoll: packet transport cannot be reconfigured")
	errUnsupportedPacketMode = errors.New("netpoll: unsupported packet transport")
	errInvalidPlainPacket    = errors.New("netpoll: invalid plain packet")
)

// Conn is a CloudWeGo/netpoll TCP connection with native MTProto packet
// framing. Dial returns it as a net.Conn so it remains directly assignable to
// DialFunc.
type Conn struct {
	connection cloudnetpoll.Connection
	readSpin   time.Duration

	stateMu    sync.Mutex
	readMu     sync.Mutex
	writeMu    sync.Mutex
	mode       uint8
	configured bool
	used       bool
	closed     bool
}

var _ net.Conn = (*Conn)(nil)

func newConn(connection cloudnetpoll.Connection, readSpin time.Duration) *Conn {
	return &Conn{connection: connection, readSpin: readSpin}
}

// ConfigurePacketTransport selects the MTProto wire framing used by the
// native packet methods. Repeating the selected mode is idempotent; changing
// it, or configuring a connection already used as a byte stream, is rejected.
func (connection *Conn) ConfigurePacketTransport(mode uint8) error {
	if connection == nil || connection.connection == nil {
		return net.ErrClosed
	}
	if mode > transportPaddedIntermediate {
		return fmt.Errorf("%w: %d", errUnsupportedPacketMode, mode)
	}

	connection.stateMu.Lock()
	defer connection.stateMu.Unlock()
	if connection.closed {
		return net.ErrClosed
	}
	if connection.configured {
		if connection.mode == mode {
			return nil
		}
		return fmt.Errorf("%w: %d to %d", errPacketReconfiguration, connection.mode, mode)
	}
	if connection.used {
		return errPacketTransportInUse
	}
	connection.mode = mode
	connection.configured = true
	return nil
}

func (connection *Conn) packetMode() (uint8, error) {
	if connection == nil || connection.connection == nil {
		return 0, net.ErrClosed
	}
	connection.stateMu.Lock()
	defer connection.stateMu.Unlock()
	if connection.closed {
		return 0, net.ErrClosed
	}
	if !connection.configured {
		return 0, errPacketNotConfigured
	}
	connection.used = true
	return connection.mode, nil
}

func (connection *Conn) markUsed() error {
	if connection == nil || connection.connection == nil {
		return net.ErrClosed
	}
	connection.stateMu.Lock()
	defer connection.stateMu.Unlock()
	if connection.closed {
		return net.ErrClosed
	}
	connection.used = true
	return nil
}

// Read implements net.Conn. Packet-aware users should prefer ReadPacket or
// ReadPacketView after configuring the transport.
func (connection *Conn) Read(buffer []byte) (int, error) {
	connection.readMu.Lock()
	defer connection.readMu.Unlock()
	if err := connection.markUsed(); err != nil {
		return 0, err
	}
	return connection.connection.Read(buffer)
}

// Write implements net.Conn. It is also used for the initial transport marker
// immediately after ConfigurePacketTransport.
func (connection *Conn) Write(buffer []byte) (int, error) {
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	if err := connection.markUsed(); err != nil {
		return 0, err
	}
	return connection.connection.Write(buffer)
}

func (connection *Conn) Close() error {
	if connection == nil || connection.connection == nil {
		return net.ErrClosed
	}
	connection.stateMu.Lock()
	if connection.closed {
		connection.stateMu.Unlock()
		return nil
	}
	connection.closed = true
	connection.used = true
	underlying := connection.connection
	connection.stateMu.Unlock()
	return underlying.Close()
}

func (connection *Conn) LocalAddr() net.Addr {
	if connection == nil || connection.connection == nil {
		return nil
	}
	return connection.connection.LocalAddr()
}

func (connection *Conn) RemoteAddr() net.Addr {
	if connection == nil || connection.connection == nil {
		return nil
	}
	return connection.connection.RemoteAddr()
}

func (connection *Conn) SetDeadline(deadline time.Time) error {
	if connection == nil || connection.connection == nil {
		return net.ErrClosed
	}
	return connection.connection.SetDeadline(deadline)
}

func (connection *Conn) SetReadDeadline(deadline time.Time) error {
	if connection == nil || connection.connection == nil {
		return net.ErrClosed
	}
	return connection.connection.SetReadDeadline(deadline)
}

func (connection *Conn) SetWriteDeadline(deadline time.Time) error {
	if connection == nil || connection.connection == nil {
		return net.ErrClosed
	}
	return connection.connection.SetWriteDeadline(deadline)
}

// ReadPacketView reads one packet and lends its payload to callback. The slice
// is backed by netpoll's reader and is released immediately after callback
// returns; callback must not retain it.
func (connection *Conn) ReadPacketView(maxPayload int, callback func([]byte) error) error {
	if callback == nil {
		return errInvalidPacket
	}
	connection.readMu.Lock()
	defer connection.readMu.Unlock()
	mode, err := connection.packetMode()
	if err != nil {
		return err
	}
	return connection.readFrameView(mode, maxPayload, true, callback)
}

// ReadPacket returns an owned copy of one framed MTProto payload.
func (connection *Conn) ReadPacket(maxPayload int) ([]byte, error) {
	var packet []byte
	err := connection.ReadPacketView(maxPayload, func(view []byte) error {
		packet = append([]byte(nil), view...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return packet, nil
}

// ReadPlainPacket returns an owned, validated plain MTProto envelope. Padded
// intermediate transport padding is removed before the buffer is returned.
func (connection *Conn) ReadPlainPacket(maxPayload int) ([]byte, error) {
	connection.readMu.Lock()
	defer connection.readMu.Unlock()
	mode, err := connection.packetMode()
	if err != nil {
		return nil, err
	}

	limit := normalizeMaxPayload(maxPayload)
	var packet []byte
	err = connection.readFrameView(mode, limit, false, func(view []byte) error {
		length, err := plainPacketLength(view, limit, mode == transportPaddedIntermediate)
		if err != nil {
			return err
		}
		packet = append([]byte(nil), view[:length]...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return packet, nil
}

func (connection *Conn) readFrameView(
	mode uint8,
	maxPayload int,
	trimPadded bool,
	callback func([]byte) error,
) error {
	reader := connection.connection.Reader()
	limit := normalizeMaxPayload(maxPayload)
	spinForPacketHeader(reader, mode, connection.readSpin)
	var length uint64

	switch mode {
	case transportIntermediate:
		value, err := readUint32(reader)
		if err != nil {
			return fmt.Errorf("netpoll: read intermediate header: %w", err)
		}
		length = uint64(value)
		if length == 0 || length%4 != 0 {
			return errInvalidPacketLength
		}
		if length > uint64(limit) {
			return errPacketLimit
		}
	case transportAbridged:
		words, err := readAbridgedWords(reader)
		if err != nil {
			return fmt.Errorf("netpoll: read abridged header: %w", err)
		}
		if words == 0 {
			return errInvalidPacketLength
		}
		length = uint64(words) * 4
		if length > uint64(limit) {
			return errPacketLimit
		}
	case transportPaddedIntermediate:
		value, err := readUint32(reader)
		if err != nil {
			return fmt.Errorf("netpoll: read padded intermediate header: %w", err)
		}
		length = uint64(value)
		if length == 0 || length > paddedReadLimit(limit) {
			return errPacketLimit
		}
	default:
		return fmt.Errorf("%w: %d", errUnsupportedPacketMode, mode)
	}

	if length > uint64(maxInt()) {
		return errPacketLimit
	}
	return withReaderView(reader, int(length), func(view []byte) error {
		if mode == transportPaddedIntermediate && trimPadded && len(view) != 4 {
			payloadLength := len(view) - len(view)%16
			if payloadLength == 0 {
				return errInvalidPacketLength
			}
			view = view[:payloadLength]
		}
		return callback(view)
	})
}

func readUint32(reader cloudnetpoll.Reader) (value uint32, err error) {
	err = withReaderView(reader, 4, func(view []byte) error {
		value = binary.LittleEndian.Uint32(view)
		return nil
	})
	return value, err
}

func readAbridgedWords(reader cloudnetpoll.Reader) (words uint32, err error) {
	err = withReaderView(reader, 1, func(view []byte) error {
		words = uint32(view[0])
		return nil
	})
	if err != nil || words != 0x7f {
		return words, err
	}
	err = withReaderView(reader, 3, func(view []byte) error {
		words = uint32(view[0]) | uint32(view[1])<<8 | uint32(view[2])<<16
		return nil
	})
	return words, err
}

func withReaderView(reader cloudnetpoll.Reader, length int, callback func([]byte) error) (err error) {
	view, err := reader.Next(length)
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := reader.Release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()
	return callback(view)
}

func spinForPacketHeader(reader cloudnetpoll.Reader, mode uint8, duration time.Duration) {
	if duration <= 0 {
		return
	}
	headerLength := 4
	if mode == transportAbridged {
		headerLength = 1
	}
	if reader.Len() >= headerLength {
		return
	}

	deadline := time.Now().Add(duration)
	for reader.Len() < headerLength && time.Now().Before(deadline) {
		runtime.Gosched()
	}
}

func normalizeMaxPayload(maxPayload int) int {
	if maxPayload <= 0 {
		return defaultMaxPayload
	}
	return maxPayload
}

func paddedReadLimit(maxPayload int) uint64 {
	limit := uint64(maxPayload)
	maximum := uint64(^uint32(0)) - 15
	if limit > maximum {
		limit = maximum
	}
	return limit + 15
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func plainPacketLength(packet []byte, maxPayload int, padded bool) (int, error) {
	if len(packet) < 20 || binary.LittleEndian.Uint64(packet[:8]) != 0 ||
		binary.LittleEndian.Uint64(packet[8:16]) == 0 {
		return 0, errInvalidPlainPacket
	}
	bodyLength := uint64(binary.LittleEndian.Uint32(packet[16:20]))
	if bodyLength == 0 || bodyLength%4 != 0 {
		return 0, errInvalidPlainPacket
	}
	length := uint64(20) + bodyLength
	if length > uint64(maxPayload) || length > uint64(len(packet)) {
		return 0, errInvalidPlainPacket
	}
	trailing := uint64(len(packet)) - length
	if trailing != 0 && (!padded || trailing > 15) {
		return 0, errInvalidPlainPacket
	}
	return int(length), nil
}

// WritePacket writes one framed packet through netpoll's native Writer. The
// caller's payload is copied into writer-owned memory before the single Flush.
func (connection *Conn) WritePacket(payload []byte) error {
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	mode, err := connection.packetMode()
	if err != nil {
		return err
	}
	return connection.writeFramedPayload(mode, payload)
}

// WritePacketReserved frames the payload beginning at payloadOffset. The
// reserved caller buffer is never retained by netpoll.
func (connection *Conn) WritePacketReserved(packet []byte, payloadOffset int) error {
	if payloadOffset < 0 || payloadOffset >= len(packet) {
		return errInvalidPacket
	}
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	mode, err := connection.packetMode()
	if err != nil {
		return err
	}
	if mode == transportIntermediate && payloadOffset < 4 {
		return errInvalidPacket
	}
	return connection.writeFramedPayload(mode, packet[payloadOffset:])
}

// WritePlainPacket writes a complete unencrypted MTProto envelope with the
// selected packet framing and one native Writer flush.
func (connection *Conn) WritePlainPacket(messageID uint64, body []byte) error {
	if messageID == 0 || len(body) == 0 || len(body)%4 != 0 || len(body) > defaultMaxPayload-20 {
		return errInvalidPlainPacket
	}
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	mode, err := connection.packetMode()
	if err != nil {
		return err
	}

	payloadLength := 20 + len(body)
	spec, err := makeFrameSpec(mode, payloadLength)
	if err != nil {
		return err
	}
	writer := connection.connection.Writer()
	frame, err := writer.Malloc(spec.totalLength(payloadLength))
	if err != nil {
		return fmt.Errorf("netpoll: allocate plain packet: %w", err)
	}
	payloadOffset := spec.writeHeader(frame, payloadLength)
	payload := frame[payloadOffset : payloadOffset+payloadLength]
	clear(payload[:8])
	binary.LittleEndian.PutUint64(payload[8:16], messageID)
	binary.LittleEndian.PutUint32(payload[16:20], uint32(len(body)))
	copy(payload[20:], body)
	copy(frame[payloadOffset+payloadLength:], spec.padding[:spec.paddingLength])
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("netpoll: flush plain packet: %w", err)
	}
	return nil
}

func (connection *Conn) writeFramedPayload(mode uint8, payload []byte) error {
	spec, err := makeFrameSpec(mode, len(payload))
	if err != nil {
		return err
	}
	writer := connection.connection.Writer()
	frame, err := writer.Malloc(spec.totalLength(len(payload)))
	if err != nil {
		return fmt.Errorf("netpoll: allocate packet: %w", err)
	}
	payloadOffset := spec.writeHeader(frame, len(payload))
	copy(frame[payloadOffset:payloadOffset+len(payload)], payload)
	copy(frame[payloadOffset+len(payload):], spec.padding[:spec.paddingLength])
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("netpoll: flush packet: %w", err)
	}
	return nil
}

type frameSpec struct {
	mode          uint8
	headerLength  int
	paddingLength int
	padding       [15]byte
}

func makeFrameSpec(mode uint8, payloadLength int) (frameSpec, error) {
	spec := frameSpec{mode: mode}
	if payloadLength <= 0 {
		return spec, errInvalidPacketLength
	}

	switch mode {
	case transportIntermediate:
		if payloadLength%4 != 0 || uint64(payloadLength) > uint64(^uint32(0)) {
			return spec, errInvalidPacketLength
		}
		spec.headerLength = 4
	case transportAbridged:
		if payloadLength%4 != 0 {
			return spec, errInvalidPacketLength
		}
		words := payloadLength / 4
		if words > 0xffffff {
			return spec, errInvalidPacketLength
		}
		if words < 0x7f {
			spec.headerLength = 1
		} else {
			spec.headerLength = 4
		}
	case transportPaddedIntermediate:
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return spec, fmt.Errorf("netpoll: generate packet padding: %w", err)
		}
		spec.headerLength = 4
		spec.paddingLength = int(random[0] & 15)
		copy(spec.padding[:], random[1:1+spec.paddingLength])
		if uint64(payloadLength)+uint64(spec.paddingLength) > uint64(^uint32(0)) {
			return spec, errInvalidPacketLength
		}
	default:
		return spec, fmt.Errorf("%w: %d", errUnsupportedPacketMode, mode)
	}

	if uint64(payloadLength)+uint64(spec.headerLength)+uint64(spec.paddingLength) > uint64(maxInt()) {
		return spec, errInvalidPacketLength
	}
	return spec, nil
}

func (spec frameSpec) totalLength(payloadLength int) int {
	return spec.headerLength + payloadLength + spec.paddingLength
}

func (spec frameSpec) writeHeader(frame []byte, payloadLength int) int {
	switch spec.mode {
	case transportAbridged:
		words := payloadLength / 4
		if spec.headerLength == 1 {
			frame[0] = byte(words)
		} else {
			frame[0] = 0x7f
			frame[1] = byte(words)
			frame[2] = byte(words >> 8)
			frame[3] = byte(words >> 16)
		}
	default:
		binary.LittleEndian.PutUint32(frame[:4], uint32(payloadLength+spec.paddingLength))
	}
	return spec.headerLength
}
