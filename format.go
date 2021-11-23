package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type format struct{}

func (ft *format) Bytesize(s string) (int64, error) {
	var multiplier int64 = 1
	u := s[len(s)-1]

	if strings.ToUpper(string(u)) == "B" {
		s = s[:len(s)-1]
	}

	u = s[len(s)-1]
	switch strings.ToUpper(string(u)) {
	case "K":
		multiplier = 1024
		s = s[:len(s)-1]
	case "M":
		multiplier = 1048576
		s = s[:len(s)-1]
	case "G":
		multiplier = 1073741824
		s = s[:len(s)-1]
	case "T":
		multiplier = 1099511627776
		s = s[:len(s)-1]
	}

	v, err := strconv.ParseInt(s, 10, 0)
	if err != nil {
		return 0, err
	}

	v *= multiplier
	return v, nil
}

func (log *format) FileMode(s string) (os.FileMode, error) {
	if len(s) != 4 {
		return 0664, fmt.Errorf("invalid file mode")
	}

	o := os.FileMode(s[3])
	o = o - 48

	g := os.FileMode(s[2])
	g = g - 48

	u := os.FileMode(s[1])
	u = u - 48

	v := ((u & 7) << 6) | ((g & 7) << 3) | (o & 7)

	return os.FileMode(v), nil
}

func (ft *format) Time(s string) (time.Duration, error) {
	var unit string
	var i int = len(s) - 1
	for ; i >= 0; i-- {
		u := s[i]
		if u > 47 && u < 58 {
			i++
			break
		}
	}

	unit = string(s[i:])
	s = s[:i]

	v, err := strconv.ParseUint(s, 10, 0)
	if err != nil {
		return 0, err
	}
	t := time.Duration(v)

	switch unit {
	case "ns":
		t *= time.Nanosecond
	case "us":
		fallthrough
	case "µs":
		t *= time.Microsecond
	case "ms":
		t *= time.Millisecond
	case "s":
		fallthrough
	case "S":
		t *= time.Second
	case "min":
		t *= 60 * time.Second
	case "H":
		fallthrough
	case "h":
		t *= 3600 * time.Second
	case "day":
		fallthrough
	case "D":
		fallthrough
	case "d":
		t *= 86400 * time.Second
	case "week":
		fallthrough
	case "w":
		fallthrough
	case "W":
		t *= 604800 * time.Second
	case "month":
		fallthrough
	case "m":
		fallthrough
	case "M":
		t *= 2678400 * time.Second
	case "year":
		fallthrough
	case "y":
		fallthrough
	case "Y":
		t *= 31536000 * time.Second
	case "":
		t *= time.Second
	default:
		return 0, fmt.Errorf("invalid time unit")
	}

	return t, nil
}
