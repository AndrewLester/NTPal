package ntpal

import (
	"bufio"
	"log"
	"os"
	"strconv"
)

func readDriftInfo(system *NTPalSystem) float64 {
	file, err := os.Open(system.drift)
	if err != nil {
		return 0
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	text, _ := reader.ReadString('\n')
	text = text[:len(text)-1]
	frequency, err := strconv.ParseFloat(text, 64)
	if err != nil {
		log.Fatal("NTP drift file invalid. Delete: ", system.drift)
	}

	return frequency
}

func writeDriftInfo(system *NTPalSystem) {
	file, err := os.OpenFile(system.drift, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalf("Could not open or create drift file: %v", err)
	}
	defer file.Close()

	info("Writing clock freq:", system.Clock.Freq)
	_, err = file.WriteString(strconv.FormatFloat(system.Clock.Freq, 'E', -1, 64) + "\n")
	if err != nil {
		log.Fatalf("Could not write to drift file: %v", err)
	}
}
