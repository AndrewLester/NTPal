package main

import "time"

var PORT = 123        // NTP port number
var VERSION = 4       // NTP version number
var TOLERANCE = 15e-6 //frequency tolerance PHI (s/s)
var MINPOLL = 4       //minimum poll exponent (16 s)
var MAXPOLL = 17      // maximum poll exponent (36 h)
var MAXDISP = 16      // maximum dispersion (16 s)
var MINDISP = 0.005   // minimum dispersion increment (s)
var MAXDIST = 1       // distance threshold (1 s)
var MAXSTRAT = 16     // maximum stratum number

var eraLength int64 = 4_294_967_296 // 2^32
var unixEraOffset int64 = 2_208_988_800

type NTPDate struct {
	eraNumber int32
	eraOffset uint32
	fraction  uint64
}

type ReceivePacket struct {
	uint32 srcaddr   /* source (remote) address */
	uint32 dstaddr   /* destination (local) address */
	byte   version   /* version number */
	byte   leap      /* leap indicator */
	byte   mode      /* mode */
	byte   stratum   /* stratum */
	byte   poll      /* poll interval */
	s_char precision /* precision */
	tdist  rootdelay /* root delay */
	tdist  rootdisp  /* root dispersion */
	byte   refid     /* reference ID */
	tstamp reftime   /* reference time */
	tstamp org       /* origin timestamp */
	tstamp rec       /* receive timestamp */
	tstamp xmt       /* transmit timestamp */
	int32  keyid     /* key ID */
	digest mac       /* message digest */
	tstamp dst       /* destination timestamp */
}

type TransmitPacket struct {
	uint32 dstaddr   /* source (local) address */
	uint32 srcaddr   /* destination (remote) address */
	byte   version   /* version number */
	byte   leap      /* leap indicator */
	byte   mode      /* mode */
	byte   stratum   /* stratum */
	byte   poll      /* poll interval */
	s_char precision /* precision */
	tdist  rootdelay /* root delay */
	tdist  rootdisp  /* root dispersion */
	byte   refid     /* reference ID */
	tstamp reftime   /* reference time */
	tstamp org       /* origin timestamp */
	tstamp rec       /* receive timestamp */
	tstamp xmt       /* transmit timestamp */
	int    keyid     /* key ID */
	digest dgst      /* message digest */
}

func TimeToNTPDate(time time.Time) NTPDate {
	s := time.Unix() + unixEraOffset
	era := s / eraLength
	timestamp := s - era*eraLength
	// TODO: Need to handle fraction
	return NTPDate{eraNumber: int32(era), eraOffset: uint32(timestamp)}
}

func NTPDateToTime(ntpDate NTPDate) time.Time {
	s := int64(ntpDate.eraNumber)*eraLength + int64(ntpDate.eraOffset)
	// TODO: Need to handle fraction
	time := time.Unix(s, 0)
	return time
}
