package ntp

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
)

func encodeTransmitPacket(packet TransmitPacket) []byte {
	var buffer bytes.Buffer
	firstByte := (packet.leap << 6) | (packet.version << 3) | byte(packet.mode)
	binary.Write(&buffer, binary.BigEndian, firstByte)
	binary.Write(&buffer, binary.BigEndian, &packet.NTPFieldsEncoded)
	return buffer.Bytes()
}

func decodeRecvPacket(encoded []byte, clientAddr net.Addr, con net.PacketConn) (*ReceivePacket, error) {
	clientAddrPort, err := netip.ParseAddrPort(clientAddr.String())
	if err != nil {
		return nil, err
	}
	localAddrPort, err := netip.ParseAddrPort(con.LocalAddr().String())
	if err != nil {
		return nil, err
	}

	clientUDPAddr := net.UDPAddrFromAddrPort(clientAddrPort)
	localUDPAddr := net.UDPAddrFromAddrPort(localAddrPort)

	var leap, version, mode byte

	reader := bytes.NewReader(encoded)
	firstByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	leap = firstByte >> 6
	version = (firstByte >> 3) & 0b111
	mode = firstByte & 0b111

	ntpFieldsEncoded := NTPFieldsEncoded{}
	if err := binary.Read(reader, binary.BigEndian, &ntpFieldsEncoded); err != nil {
		return nil, err
	}

	return &ReceivePacket{
		srcaddr:          clientUDPAddr,
		dstaddr:          localUDPAddr,
		leap:             leap,
		version:          version,
		mode:             Mode(mode),
		NTPFieldsEncoded: ntpFieldsEncoded,
	}, nil
}
