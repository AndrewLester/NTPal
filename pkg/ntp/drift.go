package ntp

import (
	"bufio"
	"errors"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"
)

type ServerPollInterval struct {
	address string
	poll    int8
}

func readDriftInfo(system *NTPSystem) (float64, map[string]ServerPollInterval) {
	file, err := os.Open(system.drift)
	if err != nil {
		return 0, nil
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	text, _ := reader.ReadString('\n')

	frequency, err := strconv.ParseFloat(text, 64)
	if err != nil {
		log.Fatal("NTP drift file invalid. Delete:", system.drift)
	}

	serverPollIntervals := map[string]ServerPollInterval{}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		tokens := strings.Split(line, " ")
		poll, _ := strconv.Atoi(tokens[1])
		serverPollIntervals[tokens[0]] = ServerPollInterval{
			address: tokens[0],
			poll:    int8(poll),
		}
	}

	return frequency, serverPollIntervals
}

func writeDriftInfo(system *NTPSystem) {
	file, err := os.Open(system.drift)
	if errors.Is(err, fs.ErrExist) {
		file, err = os.Create(system.drift)
		if err != nil {
			log.Fatalf("Could not create drift file: %v", err)
		}
	} else if err != nil {
		log.Fatalf("Could not open drift file: %v", err)
	} else {
		file.Truncate(0)
		file.Seek(0, 0)
	}
	defer file.Close()

	debug("Writing clock freq:", system.clock.freq)
	_, err = file.WriteString(strconv.FormatFloat(system.clock.freq, 'E', -1, 64) + "\n")
	if err != nil {
		log.Fatalf("Could not write to drift file: %v", err)
	}

	for _, association := range system.associations {
		file.WriteString(association.srcaddr.IP.String() + " " + strconv.Itoa(int(association.hpoll)) + "\n")
	}
}
