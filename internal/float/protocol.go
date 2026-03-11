package float

import (
	"encoding/binary"
	"fmt"
)

// Frame type constants define the binary protocol for USB passthrough over WebSocket.
const (
	FrameEnumerate       byte = 0x01
	FrameEnumResult      byte = 0x02
	FrameOpen            byte = 0x03
	FrameOpenResult      byte = 0x04
	FrameClose           byte = 0x05
	FrameCloseResult     byte = 0x06
	FrameTransferOut     byte = 0x10
	FrameTransferIn      byte = 0x11
	FrameTransferResult  byte = 0x12
	FrameInterrupt       byte = 0x20
	FrameInterruptResult byte = 0x21
	FramePing            byte = 0xFE
	FramePong            byte = 0xFF
	FrameError           byte = 0xE0
)

// frameHeaderSize is the fixed size of a frame header: 1 byte type + 4 bytes length.
const frameHeaderSize = 5

// USBDevice represents a USB device detected on the client host.
type USBDevice struct {
	VendorID     uint16 `json:"vendor_id"`
	ProductID    uint16 `json:"product_id"`
	DeviceID     uint16 `json:"device_id"`
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
	SerialNumber string `json:"serial_number"`
	Class        byte   `json:"class"`
	SubClass     byte   `json:"sub_class"`
}

// deviceFixedSize is the fixed portion of a serialized USBDevice:
// VendorID(2) + ProductID(2) + DeviceID(2) + Class(1) + SubClass(1) + 3 string lengths (2 each) = 14
const deviceFixedSize = 14

// EncodeFrame builds a binary frame: [type:1][length:4 big-endian][payload:N].
func EncodeFrame(frameType byte, payload []byte) []byte {
	frame := make([]byte, frameHeaderSize+len(payload))
	frame[0] = frameType
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[frameHeaderSize:], payload)
	return frame
}

// DecodeFrame parses a binary frame into its type and payload.
func DecodeFrame(data []byte) (frameType byte, payload []byte, err error) {
	if len(data) < frameHeaderSize {
		return 0, nil, fmt.Errorf("frame too short: need at least %d bytes, got %d", frameHeaderSize, len(data))
	}

	frameType = data[0]
	length := binary.BigEndian.Uint32(data[1:5])

	if uint32(len(data)-frameHeaderSize) < length {
		return 0, nil, fmt.Errorf("frame payload truncated: header says %d bytes, have %d", length, len(data)-frameHeaderSize)
	}

	payload = make([]byte, length)
	copy(payload, data[frameHeaderSize:frameHeaderSize+int(length)])
	return frameType, payload, nil
}

// encodeString writes a length-prefixed string (2-byte big-endian length + bytes).
func encodeString(buf []byte, offset int, s string) int {
	b := []byte(s)
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(b)))
	offset += 2
	copy(buf[offset:], b)
	return offset + len(b)
}

// decodeString reads a length-prefixed string from the buffer.
func decodeString(data []byte, offset int) (string, int, error) {
	if offset+2 > len(data) {
		return "", 0, fmt.Errorf("string length truncated at offset %d", offset)
	}
	slen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+slen > len(data) {
		return "", 0, fmt.Errorf("string data truncated at offset %d: need %d bytes", offset, slen)
	}
	s := string(data[offset : offset+slen])
	return s, offset + slen, nil
}

// serializeDevice serializes a single USBDevice into bytes.
func serializeDevice(dev USBDevice) []byte {
	mfr := []byte(dev.Manufacturer)
	prod := []byte(dev.Product)
	ser := []byte(dev.SerialNumber)

	size := deviceFixedSize + len(mfr) + len(prod) + len(ser)
	buf := make([]byte, size)

	binary.BigEndian.PutUint16(buf[0:], dev.VendorID)
	binary.BigEndian.PutUint16(buf[2:], dev.ProductID)
	binary.BigEndian.PutUint16(buf[4:], dev.DeviceID)
	buf[6] = dev.Class
	buf[7] = dev.SubClass

	off := 8
	off = encodeString(buf, off, dev.Manufacturer)
	off = encodeString(buf, off, dev.Product)
	_ = encodeString(buf, off, dev.SerialNumber)

	return buf
}

// EncodeDeviceList serializes a slice of USBDevices for a FrameEnumResult payload.
// Format: [count:2 big-endian][device...]
func EncodeDeviceList(devices []USBDevice) []byte {
	// First pass: serialize each device to compute total size
	serialized := make([][]byte, len(devices))
	totalSize := 2 // 2 bytes for count
	for i, dev := range devices {
		serialized[i] = serializeDevice(dev)
		totalSize += len(serialized[i])
	}

	buf := make([]byte, totalSize)
	binary.BigEndian.PutUint16(buf[0:], uint16(len(devices)))
	off := 2
	for _, s := range serialized {
		copy(buf[off:], s)
		off += len(s)
	}

	return buf
}

// DecodeDeviceList deserializes a FrameEnumResult payload into a slice of USBDevices.
func DecodeDeviceList(data []byte) ([]USBDevice, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("device list too short: need at least 2 bytes")
	}

	count := int(binary.BigEndian.Uint16(data[0:]))
	off := 2

	devices := make([]USBDevice, 0, count)
	for i := 0; i < count; i++ {
		if off+8 > len(data) {
			return nil, fmt.Errorf("device %d: fixed fields truncated at offset %d", i, off)
		}

		dev := USBDevice{
			VendorID:  binary.BigEndian.Uint16(data[off:]),
			ProductID: binary.BigEndian.Uint16(data[off+2:]),
			DeviceID:  binary.BigEndian.Uint16(data[off+4:]),
			Class:     data[off+6],
			SubClass:  data[off+7],
		}
		off += 8

		var err error
		dev.Manufacturer, off, err = decodeString(data, off)
		if err != nil {
			return nil, fmt.Errorf("device %d manufacturer: %w", i, err)
		}
		dev.Product, off, err = decodeString(data, off)
		if err != nil {
			return nil, fmt.Errorf("device %d product: %w", i, err)
		}
		dev.SerialNumber, off, err = decodeString(data, off)
		if err != nil {
			return nil, fmt.Errorf("device %d serial: %w", i, err)
		}

		devices = append(devices, dev)
	}

	return devices, nil
}

// EncodeTransfer serializes a USB transfer payload.
// Format: [deviceID:2][endpoint:1][data:N]
func EncodeTransfer(deviceID uint16, endpoint byte, data []byte) []byte {
	buf := make([]byte, 3+len(data))
	binary.BigEndian.PutUint16(buf[0:], deviceID)
	buf[2] = endpoint
	copy(buf[3:], data)
	return buf
}

// DecodeTransfer deserializes a USB transfer payload.
func DecodeTransfer(data []byte) (deviceID uint16, endpoint byte, transferData []byte, err error) {
	if len(data) < 3 {
		return 0, 0, nil, fmt.Errorf("transfer payload too short: need at least 3 bytes, got %d", len(data))
	}

	deviceID = binary.BigEndian.Uint16(data[0:])
	endpoint = data[2]
	transferData = make([]byte, len(data)-3)
	copy(transferData, data[3:])
	return deviceID, endpoint, transferData, nil
}

// EncodeError serializes an error response payload.
// Format: [code:2 big-endian][message:UTF-8 bytes]
func EncodeError(code uint16, message string) []byte {
	msg := []byte(message)
	buf := make([]byte, 2+len(msg))
	binary.BigEndian.PutUint16(buf[0:], code)
	copy(buf[2:], msg)
	return buf
}

// DecodeError deserializes an error response payload.
func DecodeError(data []byte) (code uint16, message string) {
	if len(data) < 2 {
		return 0, ""
	}
	code = binary.BigEndian.Uint16(data[0:])
	message = string(data[2:])
	return code, message
}
