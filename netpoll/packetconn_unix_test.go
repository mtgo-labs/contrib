//go:build !windows

package netpoll

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

const packetTestTimeout = 5 * time.Second

type packetModeCase struct {
	name   string
	mode   uint8
	marker []byte
}

func packetModeCases() []packetModeCase {
	return []packetModeCase{
		{name: "intermediate", mode: transportIntermediate, marker: []byte{0xee, 0xee, 0xee, 0xee}},
		{name: "abridged", mode: transportAbridged, marker: []byte{0xef}},
		{name: "padded_intermediate", mode: transportPaddedIntermediate, marker: []byte{0xdd, 0xdd, 0xdd, 0xdd}},
	}
}

func TestPacketTransportWireModes(t *testing.T) {
	for _, testCase := range packetModeCases() {
		t.Run(testCase.name, func(t *testing.T) {
			client, server := dialPacketTestPair(t)
			if err := client.ConfigurePacketTransport(testCase.mode); err != nil {
				t.Fatalf("configure packet transport: %v", err)
			}

			writeAll(t, client, testCase.marker)

			payloadLength := 16
			if testCase.mode == transportAbridged {
				payloadLength = 0x7f * 4
			}
			payload := patternedPacket(payloadLength, 0x21)
			expectedPayload := bytes.Clone(payload)
			if err := client.WritePacket(payload); err != nil {
				t.Fatalf("write packet: %v", err)
			}
			clear(payload)

			marker := readExactly(t, server, len(testCase.marker))
			if !bytes.Equal(marker, testCase.marker) {
				t.Fatalf("marker = %x, want %x", marker, testCase.marker)
			}
			assertWrittenPacket(t, server, testCase.mode, expectedPayload)

			reservedPayload := patternedPacket(16, 0x63)
			reservedPacket := append([]byte{0xfa, 0xfb, 0xfc, 0xfd}, reservedPayload...)
			expectedReservedPayload := bytes.Clone(reservedPayload)
			if err := client.WritePacketReserved(reservedPacket, 4); err != nil {
				t.Fatalf("write reserved packet: %v", err)
			}
			clear(reservedPacket)
			assertWrittenPacket(t, server, testCase.mode, expectedReservedPayload)

			inboundPayload := patternedPacket(payloadLength, 0x91)
			var padding []byte
			if testCase.mode == transportPaddedIntermediate {
				padding = patternedPacket(11, 0xe0)
			}
			writeWireFrame(t, server, testCase.mode, inboundPayload, padding)
			packet, err := client.ReadPacket(len(inboundPayload))
			if err != nil {
				t.Fatalf("read packet: %v", err)
			}
			if !bytes.Equal(packet, inboundPayload) {
				t.Fatalf("packet = %x, want %x", packet, inboundPayload)
			}
		})
	}
}

func TestReadPaddedIntermediateTrimsEncryptedPaddingAndKeepsTransportError(t *testing.T) {
	client, server := dialPacketTestPair(t)
	if err := client.ConfigurePacketTransport(transportPaddedIntermediate); err != nil {
		t.Fatalf("configure packet transport: %v", err)
	}

	encrypted := patternedPacket(16, 0x31)
	writeWireFrame(t, server, transportPaddedIntermediate, encrypted, patternedPacket(15, 0xb0))
	packet, err := client.ReadPacket(len(encrypted))
	if err != nil {
		t.Fatalf("read padded encrypted packet: %v", err)
	}
	if !bytes.Equal(packet, encrypted) {
		t.Fatalf("packet = %x, want encrypted payload %x", packet, encrypted)
	}

	transportErrorValue := int32(-404)
	transportError := make([]byte, 4)
	binary.LittleEndian.PutUint32(transportError, uint32(transportErrorValue))
	writeWireFrame(t, server, transportPaddedIntermediate, transportError, nil)
	packet, err = client.ReadPacket(len(transportError))
	if err != nil {
		t.Fatalf("read padded transport error: %v", err)
	}
	if !bytes.Equal(packet, transportError) {
		t.Fatalf("transport error = %x, want %x", packet, transportError)
	}
	if value := int32(binary.LittleEndian.Uint32(packet)); value != transportErrorValue {
		t.Fatalf("transport error value = %d, want %d", value, transportErrorValue)
	}
}

func TestPlainPacketsAcrossWireModes(t *testing.T) {
	const messageID = uint64(0x0102030405060708)

	for _, testCase := range packetModeCases() {
		t.Run(testCase.name, func(t *testing.T) {
			client, server := dialPacketTestPair(t)
			if err := client.ConfigurePacketTransport(testCase.mode); err != nil {
				t.Fatalf("configure packet transport: %v", err)
			}

			body := patternedPacket(12, 0x42)
			expectedEnvelope := plainEnvelope(messageID, body)
			if err := client.WritePlainPacket(messageID, body); err != nil {
				t.Fatalf("write plain packet: %v", err)
			}
			clear(body)
			assertWrittenPacket(t, server, testCase.mode, expectedEnvelope)

			inboundBody := patternedPacket(8, 0x84)
			inboundEnvelope := plainEnvelope(messageID+4, inboundBody)
			var padding []byte
			if testCase.mode == transportPaddedIntermediate {
				padding = patternedPacket(13, 0xd1)
			}
			writeWireFrame(t, server, testCase.mode, inboundEnvelope, padding)
			packet, err := client.ReadPlainPacket(len(inboundEnvelope))
			if err != nil {
				t.Fatalf("read plain packet: %v", err)
			}
			if !bytes.Equal(packet, inboundEnvelope) {
				t.Fatalf("plain packet = %x, want unpadded envelope %x", packet, inboundEnvelope)
			}
		})
	}
}

func TestReadPlainPacketRejectsInvalidEnvelope(t *testing.T) {
	valid := plainEnvelope(0x0102030405060708, patternedPacket(8, 0x30))

	invalidAuthKey := bytes.Clone(valid)
	binary.LittleEndian.PutUint64(invalidAuthKey[:8], 1)
	missingMessageID := bytes.Clone(valid)
	clear(missingMessageID[8:16])
	missingBody := plainEnvelope(0x0102030405060708, nil)
	unalignedBody := bytes.Clone(valid)
	binary.LittleEndian.PutUint32(unalignedBody[16:20], 6)
	truncatedBody := bytes.Clone(valid)
	binary.LittleEndian.PutUint32(truncatedBody[16:20], 12)
	unpaddedTrailing := append(bytes.Clone(valid), 0, 0, 0, 0)
	tooMuchPaddedTrailing := append(bytes.Clone(valid), make([]byte, 16)...)

	tests := []struct {
		name    string
		mode    uint8
		payload []byte
	}{
		{name: "nonzero_auth_key", mode: transportIntermediate, payload: invalidAuthKey},
		{name: "missing_message_id", mode: transportIntermediate, payload: missingMessageID},
		{name: "missing_body", mode: transportIntermediate, payload: missingBody},
		{name: "unaligned_body", mode: transportIntermediate, payload: unalignedBody},
		{name: "truncated_body", mode: transportIntermediate, payload: truncatedBody},
		{name: "trailing_bytes_without_padding", mode: transportIntermediate, payload: unpaddedTrailing},
		{name: "more_than_fifteen_padding_bytes", mode: transportPaddedIntermediate, payload: tooMuchPaddedTrailing},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			client, server := dialPacketTestPair(t)
			if err := client.ConfigurePacketTransport(testCase.mode); err != nil {
				t.Fatalf("configure packet transport: %v", err)
			}
			writeWireFrame(t, server, testCase.mode, testCase.payload, nil)
			packet, err := client.ReadPlainPacket(len(testCase.payload))
			if packet != nil {
				t.Fatalf("packet = %x, want nil", packet)
			}
			if !errors.Is(err, errInvalidPlainPacket) {
				t.Fatalf("error = %v, want %v", err, errInvalidPlainPacket)
			}
		})
	}
}

func TestPacketTransportConfigurationRules(t *testing.T) {
	t.Run("idempotent_and_not_reconfigurable", func(t *testing.T) {
		client, _ := dialPacketTestPair(t)
		if err := client.ConfigurePacketTransport(transportPaddedIntermediate + 1); !errors.Is(err, errUnsupportedPacketMode) {
			t.Fatalf("invalid mode error = %v, want %v", err, errUnsupportedPacketMode)
		}
		if err := client.ConfigurePacketTransport(transportAbridged); err != nil {
			t.Fatalf("configure packet transport: %v", err)
		}
		if err := client.ConfigurePacketTransport(transportAbridged); err != nil {
			t.Fatalf("repeat packet configuration: %v", err)
		}
		if err := client.ConfigurePacketTransport(transportIntermediate); !errors.Is(err, errPacketReconfiguration) {
			t.Fatalf("reconfiguration error = %v, want %v", err, errPacketReconfiguration)
		}
	})

	t.Run("packet_use_requires_configuration", func(t *testing.T) {
		client, _ := dialPacketTestPair(t)
		if err := client.WritePacket(patternedPacket(4, 0x22)); !errors.Is(err, errPacketNotConfigured) {
			t.Fatalf("unconfigured packet write error = %v, want %v", err, errPacketNotConfigured)
		}
		if err := client.ConfigurePacketTransport(transportIntermediate); err != nil {
			t.Fatalf("configure after rejected packet write: %v", err)
		}
	})

	t.Run("byte_stream_write_prevents_configuration", func(t *testing.T) {
		client, server := dialPacketTestPair(t)
		writeAll(t, client, []byte{0xa5})
		if data := readExactly(t, server, 1); data[0] != 0xa5 {
			t.Fatalf("stream byte = %x, want a5", data)
		}
		if err := client.ConfigurePacketTransport(transportIntermediate); !errors.Is(err, errPacketTransportInUse) {
			t.Fatalf("configuration error = %v, want %v", err, errPacketTransportInUse)
		}
	})

	t.Run("byte_stream_read_prevents_configuration", func(t *testing.T) {
		client, server := dialPacketTestPair(t)
		writeAll(t, server, []byte{0x5a})
		buffer := make([]byte, 1)
		if _, err := io.ReadFull(client, buffer); err != nil {
			t.Fatalf("stream read: %v", err)
		}
		if buffer[0] != 0x5a {
			t.Fatalf("stream byte = %x, want 5a", buffer)
		}
		if err := client.ConfigurePacketTransport(transportIntermediate); !errors.Is(err, errPacketTransportInUse) {
			t.Fatalf("configuration error = %v, want %v", err, errPacketTransportInUse)
		}
	})
}

func TestReadPacketViewPropagatesCallbackErrorAndReleasesReader(t *testing.T) {
	client, server := dialPacketTestPair(t)
	if err := client.ConfigurePacketTransport(transportIntermediate); err != nil {
		t.Fatalf("configure packet transport: %v", err)
	}

	firstPayload := patternedPacket(12, 0x19)
	secondPayload := patternedPacket(8, 0x71)
	writeWireFrame(t, server, transportIntermediate, firstPayload, nil)
	writeWireFrame(t, server, transportIntermediate, secondPayload, nil)

	callbackError := errors.New("callback failed")
	var callbackPayload []byte
	calls := 0
	err := client.ReadPacketView(len(firstPayload), func(view []byte) error {
		calls++
		callbackPayload = bytes.Clone(view)
		return callbackError
	})
	if err != callbackError {
		t.Fatalf("callback error = %v, want identical error %v", err, callbackError)
	}
	if calls != 1 {
		t.Fatalf("callback calls = %d, want 1", calls)
	}
	if !bytes.Equal(callbackPayload, firstPayload) {
		t.Fatalf("callback payload = %x, want %x", callbackPayload, firstPayload)
	}

	packet, err := client.ReadPacket(len(secondPayload))
	if err != nil {
		t.Fatalf("read after callback error: %v", err)
	}
	if !bytes.Equal(packet, secondPayload) {
		t.Fatalf("packet after callback error = %x, want %x", packet, secondPayload)
	}
}

type packetAcceptResult struct {
	connection net.Conn
	err        error
}

func dialPacketTestPair(t *testing.T) (*Conn, net.Conn) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	accepted := make(chan packetAcceptResult, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		accepted <- packetAcceptResult{connection: connection, err: acceptErr}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), packetTestTimeout)
	defer cancel()
	connection, err := Dial(ctx, listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		_ = connection.Close()
	})

	var result packetAcceptResult
	select {
	case result = <-accepted:
	case <-ctx.Done():
		t.Fatalf("accept: %v", ctx.Err())
	}
	if result.err != nil {
		t.Fatalf("accept: %v", result.err)
	}
	t.Cleanup(func() {
		_ = result.connection.Close()
	})
	_ = listener.Close()

	client, ok := connection.(*Conn)
	if !ok {
		t.Fatalf("Dial returned %T, want *Conn", connection)
	}
	deadline := time.Now().Add(packetTestTimeout)
	if err := client.SetDeadline(deadline); err != nil {
		t.Fatalf("set client deadline: %v", err)
	}
	if err := result.connection.SetDeadline(deadline); err != nil {
		t.Fatalf("set server deadline: %v", err)
	}
	return client, result.connection
}

func patternedPacket(length int, seed byte) []byte {
	packet := make([]byte, length)
	for index := range packet {
		packet[index] = seed + byte(index*17)
	}
	return packet
}

func plainEnvelope(messageID uint64, body []byte) []byte {
	packet := make([]byte, 20+len(body))
	binary.LittleEndian.PutUint64(packet[8:16], messageID)
	binary.LittleEndian.PutUint32(packet[16:20], uint32(len(body)))
	copy(packet[20:], body)
	return packet
}

func abridgedTestHeader(t *testing.T, payloadLength int) []byte {
	t.Helper()
	if payloadLength <= 0 || payloadLength%4 != 0 {
		t.Fatalf("invalid abridged test payload length %d", payloadLength)
		return nil
	}
	words := payloadLength / 4
	if words < 0x7f {
		return []byte{byte(words)}
	}
	if words > 0xffffff {
		t.Fatalf("abridged test payload is too large: %d words", words)
		return nil
	}
	return []byte{0x7f, byte(words), byte(words >> 8), byte(words >> 16)}
}

func writeWireFrame(t *testing.T, connection net.Conn, mode uint8, payload, padding []byte) {
	t.Helper()

	var frame []byte
	switch mode {
	case transportIntermediate:
		if len(padding) != 0 || len(payload) == 0 || len(payload)%4 != 0 {
			t.Fatalf("invalid intermediate test frame: payload=%d padding=%d", len(payload), len(padding))
		}
		frame = make([]byte, 4, 4+len(payload))
		binary.LittleEndian.PutUint32(frame, uint32(len(payload)))
	case transportAbridged:
		if len(padding) != 0 {
			t.Fatalf("abridged test frame has %d padding bytes", len(padding))
		}
		frame = abridgedTestHeader(t, len(payload))
	case transportPaddedIntermediate:
		if len(payload) == 0 || len(padding) > 15 {
			t.Fatalf("invalid padded test frame: payload=%d padding=%d", len(payload), len(padding))
		}
		frame = make([]byte, 4, 4+len(payload)+len(padding))
		binary.LittleEndian.PutUint32(frame, uint32(len(payload)+len(padding)))
	default:
		t.Fatalf("unsupported test mode %d", mode)
		return
	}
	frame = append(frame, payload...)
	frame = append(frame, padding...)
	writeAll(t, connection, frame)
}

func assertWrittenPacket(t *testing.T, connection net.Conn, mode uint8, expectedPayload []byte) {
	t.Helper()

	switch mode {
	case transportIntermediate:
		header := readExactly(t, connection, 4)
		if length := binary.LittleEndian.Uint32(header); uint64(length) != uint64(len(expectedPayload)) {
			t.Fatalf("intermediate length = %d, want %d", length, len(expectedPayload))
		}
		payload := readExactly(t, connection, len(expectedPayload))
		if !bytes.Equal(payload, expectedPayload) {
			t.Fatalf("intermediate payload = %x, want %x", payload, expectedPayload)
		}
	case transportAbridged:
		expectedHeader := abridgedTestHeader(t, len(expectedPayload))
		header := readExactly(t, connection, len(expectedHeader))
		if !bytes.Equal(header, expectedHeader) {
			t.Fatalf("abridged header = %x, want %x", header, expectedHeader)
		}
		payload := readExactly(t, connection, len(expectedPayload))
		if !bytes.Equal(payload, expectedPayload) {
			t.Fatalf("abridged payload = %x, want %x", payload, expectedPayload)
		}
	case transportPaddedIntermediate:
		header := readExactly(t, connection, 4)
		length := uint64(binary.LittleEndian.Uint32(header))
		minimum := uint64(len(expectedPayload))
		if length < minimum || length > minimum+15 {
			t.Fatalf("padded length = %d, want [%d,%d]", length, minimum, minimum+15)
		}
		payload := readExactly(t, connection, int(length))
		if !bytes.Equal(payload[:len(expectedPayload)], expectedPayload) {
			t.Fatalf("padded payload prefix = %x, want %x", payload[:len(expectedPayload)], expectedPayload)
		}
	default:
		t.Fatalf("unsupported test mode %d", mode)
	}
}

func writeAll(t *testing.T, connection net.Conn, data []byte) {
	t.Helper()
	for len(data) > 0 {
		count, err := connection.Write(data)
		if err != nil {
			t.Fatalf("write %d bytes: %v", len(data), err)
		}
		if count <= 0 || count > len(data) {
			t.Fatalf("write count = %d for %d bytes", count, len(data))
		}
		data = data[count:]
	}
}

func readExactly(t *testing.T, connection net.Conn, length int) []byte {
	t.Helper()
	data := make([]byte, length)
	if _, err := io.ReadFull(connection, data); err != nil {
		t.Fatalf("read %d bytes: %v", length, err)
	}
	return data
}
