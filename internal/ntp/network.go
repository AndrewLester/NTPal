package ntp

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
)

type ReceivePacket struct {
	Dst TimestampEncoded /* destination timestamp */
	TransmitPacket
}

type TransmitPacket struct {
	Dstaddr *net.UDPAddr /* source (local) address */
	Srcaddr *net.UDPAddr /* destination (remote) address */
	Leap    byte         /* leap indicator */
	Version byte         /* version number */
	Mode    Mode         /* mode */
	Keyid   int32        /* key ID */
	Dgst    Digest       /* message digest */
	NtpFieldsEncoded
}

type NtpFieldsEncoded struct {
	Stratum   byte             /* stratum */
	Poll      int8             /* poll interval */
	Precision int8             /* precision */
	Rootdelay ShortEncoded     /* root delay */
	Rootdisp  ShortEncoded     /* root dispersion */
	Refid     ShortEncoded     /* reference ID */
	Reftime   TimestampEncoded /* reference time */
	Org       TimestampEncoded /* origin timestamp */
	Rec       TimestampEncoded /* receive timestamp */
	Xmt       TimestampEncoded /* transmit timestamp */
}

func EncodeTransmitPacket(packet TransmitPacket) []byte {
	firstByte := (packet.Leap << 6) | (packet.Version << 3) | byte(packet.Mode)

	var buffer bytes.Buffer
	binary.Write(&buffer, binary.BigEndian, firstByte)
	binary.Write(&buffer, binary.BigEndian, &packet.NtpFieldsEncoded)
	return buffer.Bytes()
}

func DecodeRecvPacket(encoded []byte, clientAddr net.Addr, con net.PacketConn) (*ReceivePacket, error) {
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

	reader := bytes.NewReader(encoded)
	firstByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	leap := firstByte >> 6
	version := (firstByte >> 3) & 0b111
	mode := firstByte & 0b111

	fieldsEncoded := NtpFieldsEncoded{}
	if err := binary.Read(reader, binary.BigEndian, &fieldsEncoded); err != nil {
		return nil, err
	}

	return &ReceivePacket{
		TransmitPacket: TransmitPacket{
			Srcaddr:          clientUDPAddr,
			Dstaddr:          localUDPAddr,
			Leap:             leap,
			Version:          version,
			Mode:             Mode(mode),
			NtpFieldsEncoded: fieldsEncoded,
		},
	}, nil
}
