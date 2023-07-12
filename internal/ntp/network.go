package ntp

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
)

type ReceivePacket struct {
	Srcaddr *net.UDPAddr        /* source (remote) address */
	Dstaddr *net.UDPAddr        /* destination (local) address */
	Leap    byte                /* leap indicator */
	Version byte                /* version number */
	Mode    Mode                /* mode */
	Keyid   int32               /* key ID */
	Mac     Digest              /* message digest */
	Dst     NTPTimestampEncoded /* destination timestamp */
	ntpFieldsEncoded
}

type TransmitPacket struct {
	Dstaddr *net.UDPAddr /* source (local) address */
	Srcaddr *net.UDPAddr /* destination (remote) address */
	Leap    byte         /* leap indicator */
	Version byte         /* version number */
	Mode    Mode         /* mode */
	Keyid   int          /* key ID */
	Dgst    Digest       /* message digest */
	ntpFieldsEncoded
}

type ntpFieldsEncoded struct {
	Stratum   byte                /* stratum */
	Poll      int8                /* poll interval */
	Precision int8                /* precision */
	Rootdelay NTPShortEncoded     /* root delay */
	Rootdisp  NTPShortEncoded     /* root dispersion */
	Refid     NTPShortEncoded     /* reference ID */
	Reftime   NTPTimestampEncoded /* reference time */
	Org       NTPTimestampEncoded /* origin timestamp */
	Rec       NTPTimestampEncoded /* receive timestamp */
	Xmt       NTPTimestampEncoded /* transmit timestamp */
}

func EncodeTransmitPacket(packet TransmitPacket) []byte {
	firstByte := (packet.leap << 6) | (packet.version << 3) | byte(packet.mode)

	var buffer bytes.Buffer
	binary.Write(&buffer, binary.BigEndian, firstByte)
	binary.Write(&buffer, binary.BigEndian, &packet.ntpFieldsEncoded)
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

	fieldsEncoded := ntpFieldsEncoded{}
	if err := binary.Read(reader, binary.BigEndian, &fieldsEncoded); err != nil {
		return nil, err
	}

	return &ReceivePacket{
		srcaddr:          clientUDPAddr,
		dstaddr:          localUDPAddr,
		leap:             leap,
		version:          version,
		mode:             Mode(mode),
		ntpFieldsEncoded: fieldsEncoded,
	}, nil
}
