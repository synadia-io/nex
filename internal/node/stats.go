package nexnode

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	controlapi "github.com/synadia-io/nex/internal/control-api"
)

// This function only works on Linux, but that's okay since nex-node can only run on 64-bit linux
func ReadMemoryStats() (*controlapi.MemoryStat, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	res := controlapi.MemoryStat{}
	for scanner.Scan() {
		key, value := parseLine(scanner.Text())
		switch key {
		case "MemTotal":
			res.MemTotal = value
		case "MemFree":
			res.MemFree = value
		case "MemAvailable":
			res.MemAvailable = value
		}
	}
	return &res, nil
}

func parseLine(raw string) (key string, value int) {
	text := strings.ReplaceAll(raw[:len(raw)-2], " ", "")
	keyValue := strings.Split(text, ":")
	return keyValue[0], toInt(keyValue[1])
}

func toInt(raw string) int {
	if raw == "" {
		return 0
	}
	res, err := strconv.Atoi(raw)
	if err != nil {
		return -1
	}
	return res
}
