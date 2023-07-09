package simulator

import (
	"os"
	"time"
)

type Simulator interface {
	Get(Trace) error
	PrintToFile(file *os.File, start time.Time) error
}

type Trace struct {
	Addr int
	Op   string
}
