package ntpal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/AndrewLester/ntpal/internal/ntp"
	"github.com/AndrewLester/ntpal/internal/rpc"
)

const VERSION byte = 4     // NTP version number
const TOLERANCE = 15e-6    //frequency tolerance PHI (s/s)
const MINPOLL int8 = 3     //minimum poll exponent (8 s)
const MAXPOLL int8 = 17    // maximum poll exponent (36 h)
const MAXDISP float64 = 16 // maximum dispersion (16 s)
const MINDISP = 0.005      // minimum dispersion increment (s)
const NOSYNC byte = 0x3    // leap unsync
const MAXDIST byte = 1     // distance threshold (1 s)
const MAXSTRAT byte = 16   // maximum stratum number

const SGATE = 3     /* spike gate (clock Filter */
const BDELAY = .004 /* broadcast delay (s) */
const PHI = 15e-6   /* frequency tolerance (15 ppm) */
const NSTAGE = 8    /* clock Register stages */
const NMAX = 50     /* maximum number of peers */
const NSANE = 1     /* minimum intersection survivors */
const NMIN = 3      /* minimum cluster survivors */
const NTP_FWEIGHT = 0.5
const UNREACH = 8 /* unreach counter threshold */
const BCOUNT = 8  /* packets in a burst */
const BTIME = 2   /* burst interval (s) */

const STEPT = .128     /* step threshold (s) */
const WATCH = 600      /* stepout threshold (s) */
const PANICT = 1000    /* panic threshold (s) */
const PLL = 16         /* PLL loop gain */
const FLL = 0.25       /* FLL loop gain */
const AVG = 4          /* parameter averaging constant */
const ALLAN = 2048     /* compromise Allan intercept (s) */
const LIMIT = 30       /* poll-adjust threshold */
const MAXFREQ = 500e-6 /* frequency tolerance (500 ppm) */
const PGATE = 4        /* poll-adjust gate */

const MINCLOCK = 3  /* minimum manycast survivors */
const MAXCLOCK = 10 /* maximum manycast candidates */
const TTLMAX = 8    /* max ttl manycast */
const BEACON = 15   /* max interval between beacons */

const MTU = 1300
const PRECISION = -18 /* precision (log2 s)  */

const STARTUP_OFFSET_MAX = 5e-4

const (
	NSET int = iota
	FSET
	SPIK
	FREQ
	SYNC
)

type DispatchCode int

const ERR DispatchCode = -1
const (
	DSCRD DispatchCode = iota
	PROC
	BCST
	FXMIT
	MANY
	NEWPS
	NEWBC
)

type AssociationStateCode byte

const (
	INIT AssociationStateCode = iota
	STALE
	STEP
	ERROR
	CRYPTO
	NKEY
)

type LocalClockReturnCode int

const (
	IGNORE LocalClockReturnCode = iota
	SLEW
	LSTEP
	PANIC
)

// Index with [associationMode][packetMode]
var dispatchTable = [][]DispatchCode{
	{NEWPS, DSCRD, FXMIT, MANY, NEWBC},
	{PROC, PROC, DSCRD, DSCRD, DSCRD},
	{PROC, ERR, DSCRD, DSCRD, DSCRD},
	{DSCRD, DSCRD, DSCRD, PROC, DSCRD},
	{DSCRD, DSCRD, DSCRD, DSCRD, DSCRD},
	{DSCRD, DSCRD, DSCRD, DSCRD, DSCRD},
	{DSCRD, DSCRD, DSCRD, DSCRD, PROC},
}

type NTPalSystem struct {
	ntp.System
	Clock       Clock        // Override value of "Clock"
	Association *Association // Override value of "Association"

	address   *net.UDPAddr
	t         ntp.TimestampEncoded /* update time */
	stratum   byte                 /* stratum */
	poll      int8                 /* poll interval */
	precision int8                 /* precision */
	refid     ntp.ShortEncoded     /* reference ID */
	reftime   ntp.TimestampEncoded /* reference time */
	// Max of the two below is NMAX, but using slice type becaues it's not always full, and nils cant be sorted
	m            []Chime    /* chime list */
	v            []Survivor /* survivor list */
	n            int        /* number of survivors */
	associations []*Association
	conn         *net.UDPConn

	host string
	port string

	mode ntp.Mode

	drift  string
	config string

	wg sync.WaitGroup

	hold int64

	// QUERY

	query            bool
	filtered         chan any
	FilteredProgress chan any

	// RPC
	socket string
}

type Association struct {
	hmode ntp.Mode // HOST (Self) mode

	ntp.Association
	t float64             /* clock.T of last used sample */
	f [NSTAGE]FilterStage /* clock Filter */

	burst    int
	ttl      int
	unreach  int
	outdate  int32
	nextdate int32
	maxpoll  int8
	minpoll  int8

	isMany    bool // manycast client association
	ephemeral bool
}

type Chime struct {
	association *Association
	levelType   int
	edge        float64
}

type Chimers []Chime

func (s Chimers) Len() int      { return len(s) }
func (s Chimers) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByEdge struct {
	Chimers
}

func (s ByEdge) Less(i, j int) bool {
	return s.Chimers[i].edge < s.Chimers[j].edge
}

type Survivor struct {
	association *Association /* peer structure pointer */
	metric      float64
}

type Survivors []Survivor

func (s Survivors) Len() int      { return len(s) }
func (s Survivors) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByMetric struct {
	Survivors
}

func (s ByMetric) Less(i, j int) bool {
	return s.Survivors[i].metric < s.Survivors[j].metric
}

// "t" is process time, not realtime. Only second incrementing
type Clock struct {
	lock sync.Mutex
	ntp.Clock
}

type FilterStage struct {
	t      ntp.TimestampEncoded /* update time */
	offset float64              /* clock Ofset */
	delay  float64              /* roundtrip delay */
	disp   float64              /* dispersion */
}

type FilterStages [NSTAGE]FilterStage

func (s *FilterStages) Len() int      { return len(s) }
func (s *FilterStages) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByDelay struct {
	*FilterStages
}

func (s ByDelay) Less(i, j int) bool {
	return s.FilterStages[i].delay < s.FilterStages[j].delay
}

func NewSystem(host, port, config, drift, socket string) *NTPalSystem {
	system := &NTPalSystem{
		host:             host,
		port:             port,
		config:           config,
		socket:           socket,
		drift:            drift,
		mode:             ntp.SERVER,
		poll:             MINPOLL,
		precision:        PRECISION,
		stratum:          MAXSTRAT,
		hold:             WATCH,
		filtered:         make(chan any),
		FilteredProgress: make(chan any),
		System: ntp.System{
			Leap: NOSYNC,
		},
	}
	// Map the local struct values to the embedded System's
	system.System.Clock = &system.Clock.Clock
	return system
}

func (system *NTPalSystem) Start() {
	config := parseConfig(system.config)

	if system.drift == "" {
		system.drift = config.driftfile
	}

	freq := readDriftInfo(system)
	if freq != 0 {
		system.Clock.Freq = freq
		system.rstclock(FSET, 0, 0)
	} else {
		system.rstclock(NSET, 0, 0)
	}

	system.Clock.Jitter = ntp.Log2ToDouble(system.precision)

	rand.Seed(time.Now().UnixNano())

	address, err := net.ResolveUDPAddr("udp", net.JoinHostPort(system.host, system.port))
	if err != nil {
		log.Fatal("Could not resolve NTP_HOST + NTP_PORT")
	}
	system.address = address

	system.setupAssociations(config.servers)

	if system.mode == ntp.SERVER {
		system.listen()

		system.wg.Add(1)
		go system.setupServer()

		rpcServer := &rpc.NTPalRPCServer{Socket: system.socket, System: &system.System, GetAssociations: system.GetAssociations}

		system.wg.Add(1)
		go rpcServer.Listen(&system.wg)
	}

	system.wg.Wait()
}

func (system *NTPalSystem) setupAssociations(associationConfigs []serverAssociationConfig) {
	for _, associationConfig := range associationConfigs {
		association := &Association{
			hmode:   associationConfig.hmode,
			maxpoll: int8(associationConfig.maxpoll),
			minpoll: int8(associationConfig.minpoll),
			Association: ntp.Association{
				Hpoll:    int8(associationConfig.minpoll),
				Hostname: associationConfig.hostname,
				ReceivePacket: ntp.ReceivePacket{
					TransmitPacket: ntp.TransmitPacket{
						Srcaddr: associationConfig.address,
						Dstaddr: system.address,
						Version: byte(associationConfig.version),
					},
				},
			},
		}
		system.clear(association, INIT)

		association.BurstEnabled = associationConfig.burst
		association.IburstEnabled = associationConfig.iburst

		system.associations = append(system.associations, association)
	}

	system.wg.Add(1)

	go func() {
		for {
			system.clockAdjust()
			time.Sleep(time.Second)
		}
	}()
}

func (system *NTPalSystem) listen() {
	udp, err := net.ListenUDP("udp", system.address)
	if err != nil {
		log.Fatalf("can't listen on %v/udp: %s", system.address, err)
	}

	system.conn = udp
}

func (system *NTPalSystem) setupServer() {
	defer system.wg.Done()

	packet := make([]byte, MTU)

	for {
		_, addr, err := system.conn.ReadFrom(packet)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			log.Printf("error reading on %s/udp: %s", addr, err)
			continue
		}

		recvPacket, err := ntp.DecodeRecvPacket(packet, addr, system.conn)
		if err != nil {
			log.Printf("Error reading packet: %v", err)
		}
		recvPacket.Dst = GetSystemTime()
		reply := system.receive(*recvPacket)
		if reply == nil {
			continue
		}
		encoded := ntp.EncodeTransmitPacket(*reply)
		n, e := system.conn.WriteTo(encoded, addr)
		info("(Reply) Bytes written:", n, "Err:", e)
	}

}

func (system *NTPalSystem) clockAdjust() {
	system.Clock.lock.Lock()

	system.Clock.T++
	system.Rootdisp += PHI

	dtemp := system.Clock.Offset / (float64(PLL) * ntp.Log2ToDouble(system.poll))
	if system.Clock.State != SYNC {
		dtemp = 0
	} else if system.hold > 0 {
		dtemp = system.Clock.Offset / (float64(PLL) * ntp.Log2ToDouble(1))
		system.hold--
	}

	system.Clock.Offset -= dtemp
	debug("*****ADJUSTING:")
	debug("TIME:", system.Clock.T, "SYS OFFSET:", system.Offset, "CLOCK OFFSET:", system.Clock.Offset)
	debug("FREQ: ", system.Clock.Freq*1e6, "OFFSET (dtemp):", dtemp*1e6)
	adjustTime(system.Clock.Freq + dtemp)

	system.Clock.lock.Unlock()

	for _, association := range system.associations {
		if system.Clock.T >= ntp.TimestampEncoded(association.nextdate) {
			info("sendPoll:", association.Srcaddr.IP)
			system.sendPoll(association)
		}
	}

	// Once per hour, write the clock Frequency to a file.
	if system.Clock.T%3600 == 3599 {
		writeDriftInfo(system)
	}

	if system.Clock.T%10 == 0 {
		sysPeerIP := "NONE"
		if system.Association != nil {
			sysPeerIP = system.Association.Srcaddr.IP.String()
		}
		info("*****REPORT:")
		info(
			"(SYSTEM):",
			"T:", system.t,
			"OFFSET:", system.Offset,
			"JITTER:", system.Jitter,
			"PEER:", sysPeerIP,
			"POLL:", system.poll,
			"HOLD:", system.hold,
		)
		info(
			"(CLOCK):",
			"T:", system.Clock.T,
			"STATE:", system.Clock.State,
			"FREQ:", system.Clock.Freq,
			"OFFSET:", system.Clock.Offset,
			"JITTER:", system.Clock.Jitter,
			"WANDER:", system.Clock.Wander,
			"COUNT:", system.Clock.Count,
		)
		for _, association := range system.associations {
			ip := refIDToIP(association.Refid)
			refid := ip.String()
			if association.Stratum < 2 {
				refid = string(ip)
			}
			sync := "SYNC"
			if association.Leap == NOSYNC {
				sync = "NOSYNC"
			}
			info(
				"("+association.Srcaddr.IP.String()+"):",
				sync,
				"POLL:", strconv.Itoa(int(ntp.Log2ToDouble(association.Poll)))+"s",
				"hPOLL:", strconv.Itoa(int(ntp.Log2ToDouble(association.Hpoll)))+"s",
				"STRATUM:", association.Stratum,
				"REFID:", refid,
				"OFFSET:", association.Offset,
				"REACH:", strconv.FormatInt(int64(association.Reach), 2),
				"UNREACH:", association.unreach,
				"TIME FILTERED:", association.t,
			)
		}
	}
}

func (system *NTPalSystem) sendPoll(association *Association) {
	hpoll := association.Hpoll
	if association.hmode == ntp.BROADCAST_SERVER {
		association.outdate = int32(system.Clock.T)
		if system.Association != nil {
			system.pollPeer(association)
		}
		system.pollUpdate(association, hpoll)
		return
	}

	if association.hmode == ntp.CLIENT && association.isMany {
		association.outdate = int32(system.Clock.T)
		if association.unreach > BEACON {
			association.unreach = 0
			association.ttl = 1
			system.pollPeer(association)
		} else if system.n < MINCLOCK {
			if association.ttl < TTLMAX {
				association.ttl++
			}
			system.pollPeer(association)
		}

		association.unreach++
		system.pollUpdate(association, hpoll)
		return
	}

	if association.burst == 0 {
		association.outdate = int32(system.Clock.T)
		association.Reach = association.Reach << 1

		if association.Reach == 0 {
			system.clockFilter(association, 0, 0, MAXDISP)
			if association.IburstEnabled && association.unreach == 0 {
				association.burst = BCOUNT
			}

			if association.unreach < UNREACH {
				association.unreach++
			} else {
				// Try to get a new IP
				addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(association.Hostname, ntp.Port))
				if err != nil {
					log.Fatal("Invalid address: ", association.Hostname)
				}
				if association.Srcaddr == addr {
					hpoll++
				}
				association.Srcaddr = addr
				association.unreach = 0
			}
		} else {
			association.unreach = 0
			hpoll = system.poll
			if association.BurstEnabled && system.fit(association) {
				association.burst = BCOUNT
			}
		}
	} else {
		association.burst--
	}

	if association.hmode != ntp.BROADCAST_CLIENT {
		system.pollPeer(association)
	}
	system.pollUpdate(association, hpoll)
}

func (system *NTPalSystem) receive(packet ntp.ReceivePacket) *ntp.TransmitPacket {
	if packet.Version > VERSION {
		return nil
	}

	var association *Association
	for _, possibleAssociation := range system.associations {
		if packet.Srcaddr.IP.Equal(possibleAssociation.Srcaddr.IP) {
			association = possibleAssociation
			break
		}
	}

	hmode := ntp.RESERVED
	if association != nil {
		hmode = association.hmode
	}

	if packet.Mode > 5 {
		info("ERROR: Received packet.Mode > 5 for association with addr:", packet.Srcaddr.IP)
		info("Packet", packet)
		return nil
	}

	switch dispatchTable[hmode][packet.Mode-1] {
	case FXMIT:
		info("Received request to sync from:", packet.Srcaddr.IP)
		return system.reply(packet, ntp.SERVER)
	case NEWPS:
		if !isSymmetricEnabled() {
			return nil
		}

		association = &Association{
			Association: ntp.Association{ReceivePacket: packet},
			hmode:       ntp.SYMMETRIC_PASSIVE,
			ephemeral:   true,
		}
		system.clear(association, INIT)
		system.associations = append(system.associations, association)
	case PROC:
	default:
		return nil
	}

	info("****Processing packet From:", packet.Srcaddr.IP)

	if association == nil {
		info("Association shouldn't be nil here...")
		return nil
	}

	if packet.Xmt == 0 {
		return nil
	}

	if packet.Xmt == association.Xmt {
		return nil
	}

	unsynch := packet.Mode != ntp.BROADCAST_SERVER && (packet.Org == 0 || packet.Org != association.Xmt)

	association.Org = packet.Xmt
	association.Rec = packet.Dst

	if unsynch {
		return nil
	}

	system.process(association, packet)
	return nil
}

func (system *NTPalSystem) process(association *Association, packet ntp.ReceivePacket) {
	var offset float64
	var delay float64
	var disp float64

	kod := false

	association.Leap = packet.Leap
	association.Poll = packet.Poll
	if packet.Stratum == 0 {
		association.Stratum = MAXSTRAT

		// Process KoD code
		kod = true

		code := string(refIDToIP(packet.Refid).To4())

		switch code {
		case "DENY", "RSTR":
			associationIdx := 0
			for idx, assoc := range system.associations {
				if assoc == association {
					associationIdx = idx
					break
				}
			}
			removeIndex(&system.associations, associationIdx)
			return
		case "RATE":
			association.Poll++
			system.pollUpdate(association, association.Poll)
		default:
			kod = false
		}
	} else {
		association.Stratum = packet.Stratum
	}
	association.Mode = packet.Mode
	association.Rootdelay = float64(packet.Rootdelay) / ntp.ShortLength
	association.Rootdisp = float64(packet.Rootdisp) / ntp.ShortLength
	association.Refid = packet.Refid
	association.Reftime = packet.Reftime

	// Server must be synchronized with valid stratum
	if association.Leap == NOSYNC || association.Stratum >= MAXSTRAT {
		system.filtered <- 0
		return
	}

	if association.Rootdelay/2+association.Rootdisp >= MAXDISP || association.Reftime >
		packet.Xmt {
		return
	}

	system.pollUpdate(association, association.Hpoll)
	association.Reach |= 1

	if association.Mode == ntp.BROADCAST_SERVER {
		offset = ntp.NTPTimestampDifferenceToDouble(int64(packet.Xmt - packet.Dst))
		delay = BDELAY
		disp = ntp.Log2ToDouble(packet.Precision) + ntp.Log2ToDouble(system.precision) + PHI*
			2*BDELAY
	} else {
		offset = (ntp.NTPTimestampDifferenceToDouble(int64(packet.Rec-packet.Org)) + ntp.NTPTimestampDifferenceToDouble(int64(packet.Xmt-
			packet.Dst))) / 2
		delay = math.Max(ntp.NTPTimestampDifferenceToDouble(int64(packet.Dst-packet.Org))-ntp.NTPTimestampDifferenceToDouble(int64(packet.Xmt-
			packet.Rec)), ntp.Log2ToDouble(system.precision))
		disp = ntp.Log2ToDouble(packet.Precision) + ntp.Log2ToDouble(system.precision) + PHI*delay
	}

	// Don't use this offset/delay if KoD, probably invalid
	if kod {
		info("KoD packet, skipping filter")
		return
	}

	system.clockFilter(association, offset, delay, disp)
}

func (system *NTPalSystem) reply(receivePacket ntp.ReceivePacket, mode ntp.Mode) *ntp.TransmitPacket {
	transmitPacket := receivePacket.TransmitPacket

	transmitPacket.Srcaddr = receivePacket.Dstaddr
	transmitPacket.Dstaddr = receivePacket.Srcaddr
	transmitPacket.Leap = system.Leap
	transmitPacket.Mode = mode
	if system.stratum == MAXSTRAT {
		transmitPacket.Stratum = 0
	} else {
		transmitPacket.Stratum = system.stratum
	}
	transmitPacket.Precision = system.precision
	transmitPacket.Rootdelay = ntp.ShortEncoded(system.Rootdelay * ntp.ShortLength)
	transmitPacket.Rootdisp = ntp.ShortEncoded(system.Rootdisp * ntp.ShortLength)
	transmitPacket.Refid = system.refid
	transmitPacket.Reftime = system.reftime
	transmitPacket.Org = receivePacket.Xmt
	transmitPacket.Rec = receivePacket.Dst
	transmitPacket.Xmt = GetSystemTime()

	return &transmitPacket
}

func (system *NTPalSystem) pollPeer(association *Association) {
	transmitPacket := association.TransmitPacket

	transmitPacket.Srcaddr = association.Dstaddr
	transmitPacket.Dstaddr = association.Srcaddr
	transmitPacket.Leap = system.Leap
	transmitPacket.Mode = association.hmode
	if system.stratum == MAXSTRAT {
		transmitPacket.Stratum = 0
	} else {
		transmitPacket.Stratum = system.stratum
	}
	transmitPacket.Poll = association.Hpoll
	transmitPacket.Precision = system.precision
	transmitPacket.Rootdelay = ntp.ShortEncoded(system.Rootdelay * ntp.ShortLength)
	transmitPacket.Rootdisp = ntp.ShortEncoded(system.Rootdisp * ntp.ShortLength)
	transmitPacket.Refid = system.refid
	transmitPacket.Reftime = system.reftime
	transmitPacket.Org = association.Org
	transmitPacket.Rec = association.Rec

	transmitPacket.Xmt = GetSystemTime()
	association.Xmt = transmitPacket.Xmt

	_, err := system.conn.WriteTo(ntp.EncodeTransmitPacket(transmitPacket), transmitPacket.Dstaddr)
	if err != nil {
		info("Write to error:", err)
	}
}

func (system *NTPalSystem) pollUpdate(association *Association, poll int8) {
	association.Hpoll = int8(math.Max(math.Min(float64(association.maxpoll), float64(poll)), float64(association.minpoll)))
	if association.burst > 0 {
		if uint64(association.nextdate) != system.Clock.T {
			return
		} else {
			association.nextdate += BTIME
		}
	} else {
		association.nextdate = association.outdate + (1 << int32(math.Max(math.Min(float64(association.Poll),
			float64(association.Hpoll)), float64(MINPOLL))))
	}

	if uint64(association.nextdate) <= system.Clock.T {
		association.nextdate = int32(system.Clock.T + 1)
	}
}

func (system *NTPalSystem) clear(association *Association, kiss AssociationStateCode) {
	if system.Association == association {
		system.updateAssociation(nil)
	}

	if kiss != INIT && association.ephemeral {
		return
	}

	// Initialize the association fields for general reset.
	// Some of these are overriden below, but keep reset here for now too, as that's what the
	// RFC does.
	association.Org = 0
	association.Rec = 0
	association.Xmt = 0
	association.t = 0
	association.Update = 0
	association.f = [NSTAGE]FilterStage{}
	association.Offset = 0
	association.Delay = 0
	association.Disp = 0
	association.Jitter = 0
	association.Hpoll = 0
	association.burst = 0
	association.Reach = 0
	association.unreach = 0
	association.ttl = 0

	association.Leap = NOSYNC
	association.Stratum = MAXSTRAT
	association.Poll = association.maxpoll
	association.Hpoll = association.minpoll
	association.Disp = MAXDISP
	association.Jitter = ntp.Log2ToDouble(system.precision)
	association.Refid = uint32(kiss)
	for i := 0; i < NSTAGE; i++ {
		association.f[i].disp = MAXDISP
		association.f[i].delay = MAXDISP
	}

	association.t = float64(system.Clock.T)
	association.Update = float64(system.Clock.T)
	association.outdate = int32(association.t)
	association.nextdate = association.outdate + rand.Int31n(1<<association.minpoll)
}

func (system *NTPalSystem) clockFilter(association *Association, offset float64, delay float64, disp float64) {
	var f FilterStages

	dtemp := PHI * (float64(system.Clock.T) - association.Update)
	association.Update = float64(system.Clock.T)
	for i := NSTAGE - 1; i > 0; i-- {
		association.f[i] = association.f[i-1]
		association.f[i].disp += dtemp
		f[i] = association.f[i]
		if association.f[i].disp >= MAXDISP {
			association.f[i].disp = MAXDISP
			f[i].delay = MAXDISP
		} else if association.Update-association.t > ALLAN {
			f[i].delay = association.f[i].delay +
				association.f[i].disp
		}
	}

	association.f[0].t = system.Clock.T
	association.f[0].offset = offset
	association.f[0].delay = delay
	association.f[0].disp = disp
	f[0] = association.f[0]

	// If the clock Has stabilized, sort the samples by delay
	if system.hold == 0 {
		sort.Sort(ByDelay{&f})
	}

	m := 0
	for i := 0; i < NSTAGE; i++ {
		if f[i].delay >= MAXDISP || (m >= 2 && f[i].delay >= float64(MAXDIST)) {
			continue
		}
		m++
	}

	association.Disp = 0
	association.Jitter = 0
	for i := NSTAGE - 1; i >= 0; i-- {
		association.Disp = 0.5 * (association.Disp + f[i].disp)
		if i < m {
			association.Jitter += math.Pow((f[0].offset - f[i].offset), 2)
		}
	}

	if m == 0 {
		system.clockSelect()
		return
	}

	etemp := math.Abs(association.Offset - f[0].offset)
	association.Offset = f[0].offset
	association.Delay = f[0].delay
	if m > 1 {
		association.Jitter /= float64(m - 1)
	}
	association.Jitter = math.Max(math.Sqrt(association.Jitter), ntp.Log2ToDouble(system.precision))

	if system.query {
		system.filtered <- 0
	}

	if association.Disp < float64(MAXDIST) && f[0].disp < float64(MAXDIST) && etemp > SGATE*association.Jitter && (float64(f[0].t)-
		association.t) < float64(2*ntp.Log2ToDouble(association.Hpoll)) {
		info("Popcorn spike suppresor failed, either offset change WAY above jitter or disp too high")
		return
	}

	if float64(f[0].t) <= association.t && system.Leap != NOSYNC {
		return
	}
	association.t = float64(f[0].t)

	if association.burst == 0 || system.Leap == NOSYNC {
		system.clockSelect()
	}
}

func (system *NTPalSystem) clockSelect() {
	osys := system.Association
	system.updateAssociation(nil)

	n := 0
	system.m = []Chime{}
	for _, association := range system.associations {
		if !system.fit(association) {
			info("Association unfit:", association.Srcaddr.IP)
			continue
		}

		system.m = append(system.m, Chime{
			association: association,
			levelType:   -1,
			edge:        association.Offset - system.rootDist(association),
		})
		system.m = append(system.m, Chime{
			association: association,
			levelType:   0,
			edge:        association.Offset,
		})
		system.m = append(system.m, Chime{
			association: association,
			levelType:   1,
			edge:        association.Offset + system.rootDist(association),
		})

		n += 3
	}

	sort.Sort(ByEdge{system.m})

	m := len(system.associations)
	low := 2e9
	high := -2e9
	for allow := 0; 2*allow < m; allow++ {
		found := 0
		chime := 0
		for i := 0; i < n; i++ {
			chime -= system.m[i].levelType
			if chime >= m-allow {
				low = system.m[i].edge
				break
			}
			if system.m[i].levelType == 0 {
				found++
			}
		}

		chime = 0
		for i := n - 1; i >= 0; i-- {
			chime += system.m[i].levelType
			if chime >= m-allow {
				high = system.m[i].edge
				break
			}
			if system.m[i].levelType == 0 {
				found++
			}
		}

		if found > allow {
			continue
		}

		if high > low {
			break
		}
	}

	if high < low {
		return
	}

	system.n = 0
	system.v = []Survivor{}
	for i := 0; i < n; i++ {
		if system.m[i].edge < low || system.m[i].edge > high {
			continue
		}

		association := system.m[i].association
		system.v = append(system.v, Survivor{
			association: association,
			metric:      float64(MAXDIST*association.Stratum) + system.rootDist(association),
		})
		system.n++
	}

	if system.n < NSANE {
		return
	}

	sort.Sort(ByMetric{system.v})

	for {
		var sjitterMaxIdx int
		var max, min, dtemp float64

		max = -2e9
		min = 2e9
		for i := 0; i < system.n; i++ {
			p := system.v[i].association
			if p.Jitter < min {
				min = p.Jitter
			}
			dtemp = 0
			if system.n > 1 {
				for j := 0; j < system.n; j++ {
					q := system.v[j].association
					dtemp += math.Pow(p.Offset-q.Offset, 2)
				}
				dtemp = math.Sqrt(dtemp / float64(system.n-1))
			}

			if dtemp*system.rootDist(p) > max {
				max = dtemp
				sjitterMaxIdx = i
			}
		}

		if max < min || n <= NMIN {
			break
		}

		removeIndex(&system.v, sjitterMaxIdx)
		system.n--
	}

	if osys != nil && osys.Stratum == system.v[0].association.Stratum && containsAssociation(system.v, osys) {
		system.updateAssociation(osys)
	} else {
		system.updateAssociation(system.v[0].association)
		info("NEW SYSTEM PEER picked:", system.Association.Srcaddr.IP)
	}

	system.clockUpdate(system.Association)
}

func (system *NTPalSystem) clockUpdate(association *Association) {
	if float64(system.t) >= association.t {
		return
	}

	system.clockCombine()
	switch system.localClock(association, system.Offset) {
	case PANIC:
		debug("Offset:", system.Offset)
		log.Fatal("Offset too large!")

	case LSTEP:
		info("Discipline STEPPED")
		system.t = uint64(association.t)
		for _, association := range system.associations {
			system.clear(association, STEP)
		}
		system.stratum = MAXSTRAT
		system.poll = association.minpoll
		system.Rootdelay = 0
		system.Rootdisp = 0
		system.Jitter = ntp.Log2ToDouble(system.precision)

	case SLEW:
		info("Discipline SLEWED")
		// Offset and jitter already set by clockCombine()
		system.Leap = association.Leap
		system.t = uint64(association.t)
		system.stratum = association.Stratum + 1
		if association.Stratum == 0 || association.Stratum == 16 {
			system.refid = association.Refid
		} else {
			system.refid = ipToRefID(association.Srcaddr.IP)
		}
		system.reftime = association.Reftime
		system.Rootdelay = association.Rootdelay + association.Delay
		dtemp := math.Max(association.Rootdisp+association.Disp+system.Jitter+PHI*(float64(system.Clock.T)-association.Update)+
			math.Abs(association.Offset), MINDISP)
		system.Rootdisp = dtemp
		fmt.Println("Root disp calc:", association.Disp, association.Rootdisp)

	case IGNORE:
		info("Discipline IGNORED")
	}
}

func (system *NTPalSystem) clockCombine() {
	var w float64
	z := w
	y := z
	for i := 0; i < len(system.v) && system.v[i].association != nil; i++ {
		association := system.v[i].association
		x := system.rootDist(association)
		y += 1 / x
		z += association.Offset / x
		w += math.Pow(association.Offset-system.v[0].association.Offset, 2) / x
	}
	system.Offset = z / y
	system.Jitter = math.Sqrt(w / y)
}

func (system *NTPalSystem) localClock(association *Association, offset float64) (rval LocalClockReturnCode) {
	system.Clock.lock.Lock()
	defer system.Clock.lock.Unlock()

	if math.Abs(offset) > PANICT {
		return PANIC
	}

	rval = SLEW

	var freq float64
	mu := association.t - float64(system.t)
	info("Disciplining with offset:", offset)
	if math.Abs(offset) > STEPT {
		switch system.Clock.State {
		case SYNC:
			system.Clock.State = SPIK
			return

		case FREQ:
			if mu < WATCH {
				return IGNORE
			}

			freq = (offset - system.Clock.Offset) / mu
			fallthrough
		case SPIK:
			if mu < WATCH {
				return IGNORE
			}

			fallthrough
		default:
			stepTime(offset)
			system.Clock.Count = 0
			system.poll = association.minpoll
			rval = LSTEP
			if system.Clock.State == NSET || system.Clock.State == FSET {
				system.hold = WATCH
			}

			if system.Clock.State == NSET {
				system.rstclock(FREQ, association.t, 0)
				return
			}
		}
		system.rstclock(SYNC, association.t, 0)
	} else {
		etemp := math.Pow(system.Clock.Jitter, 2)
		dtemp := math.Pow(math.Max(math.Abs(offset-system.Clock.Last),
			ntp.Log2ToDouble(system.precision)), 2)
		system.Clock.Jitter = math.Sqrt(etemp + (dtemp-etemp)/AVG)
		switch system.Clock.State {
		case NSET:
			// Perform a step, despite offset < STEPT. The reason for this is that adjustTime
			// would mess up the frequency measurement in the next clock State.
			stepTime(offset)
			system.Clock.Count = 0
			system.poll = association.minpoll
			system.hold = WATCH
			system.rstclock(FREQ, association.t, 0)
			return LSTEP

		case FREQ:
			if mu < WATCH {
				return IGNORE
			}

			system.hold = WATCH
			freq = (offset - system.Clock.Offset) / mu

			fallthrough
		default:
			if system.hold == 0 {
				if ntp.Log2ToDouble(system.poll) > ALLAN {
					// FLL
					freq += (offset - system.Clock.Offset) / (FLL * math.Max(mu, float64(system.poll)))

					info("FREQ update (FLL):", freq)
				}

				//  PLL
				etemp = math.Min(mu, ALLAN)
				dtemp = 4 * PLL * ntp.Log2ToDouble(system.poll)
				freq += offset * etemp / (dtemp * dtemp)
				info("FREQ update (PLL):", offset*etemp/(dtemp*dtemp))
			}

			if math.Abs(offset) < STARTUP_OFFSET_MAX {
				system.hold = 0
			}
		}
		system.rstclock(SYNC, association.t, offset)
	}

	freq += system.Clock.Freq
	system.Clock.Freq = math.Max(math.Min(MAXFREQ, freq), -MAXFREQ)
	info("Set FREQ to:", system.Clock.Freq)
	etemp := math.Pow(system.Clock.Wander, 2)
	dtemp := math.Pow(freq, 2)
	system.Clock.Wander = math.Sqrt(etemp + (dtemp-etemp)/AVG)

	if system.hold > 0 {
		system.Clock.Count = 0
		return
	}

	if math.Abs(system.Clock.Offset) < PGATE*system.Clock.Jitter {
		info("Incrementing clock Count based on offset and jitter")
		system.Clock.Count += int32(system.poll)
		if system.Clock.Count > LIMIT {
			system.Clock.Count = LIMIT
			if system.poll < association.maxpoll {
				system.Clock.Count = 0
				system.poll++
			}
		}
	} else {
		system.Clock.Count -= int32(system.poll << 1)
		if system.Clock.Count < -LIMIT {
			system.Clock.Count = -LIMIT
			if system.poll > association.minpoll {
				system.Clock.Count = 0
				system.poll--
			}
		}
	}

	return
}

func (system *NTPalSystem) rstclock(state int, t, offset float64) {
	system.Clock.State = state
	system.Clock.Last = system.Clock.Offset
	system.Clock.Offset = offset
	system.t = uint64(t)
}

func (system *NTPalSystem) fit(association *Association) bool {
	if association.Leap == NOSYNC || association.Stratum >= MAXSTRAT {
		return false
	}

	if system.rootDist(association) > float64(MAXDIST)+PHI*ntp.Log2ToDouble(system.poll) {
		return false
	}

	if association.Refid == ipToRefID(association.Dstaddr.IP) || association.Refid == system.refid {
		return false
	}

	if association.Reach == 0 {
		return false
	}

	return true
}

func (system *NTPalSystem) rootDist(association *Association) float64 {
	return (association.Rootdelay+association.Delay)/2 +
		association.Rootdisp + association.Disp + PHI*float64(float64(system.Clock.T)-association.Update) + association.Jitter
}

func (system *NTPalSystem) updateAssociation(association *Association) {
	system.Association = association
	if association != nil {
		system.System.Association = &association.Association
	} else {
		system.System.Association = nil
	}
}

func (system *NTPalSystem) GetAssociations() []*ntp.Association {
	ntpAssociations := []*ntp.Association{}
	for _, association := range system.associations {
		ntpAssociations = append(ntpAssociations, &association.Association)
	}
	return ntpAssociations
}

func containsAssociation(survivors []Survivor, association *Association) bool {
	for _, survivor := range survivors {
		if survivor.association == association {
			return true
		}
	}
	return false
}

func refIDToIP(refID ntp.ShortEncoded) net.IP {
	ipBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(ipBytes, refID)
	return net.IP(ipBytes)
}

func ipToRefID(ip net.IP) ntp.ShortEncoded {
	return binary.BigEndian.Uint32(ip.To4())
}
