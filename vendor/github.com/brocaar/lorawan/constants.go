package lorawan

import (
	"time"
)

// EU863-870 default settings
const (
	ReceiveDelay1    time.Duration = time.Second
	ReceiveDelay2    time.Duration = time.Second * 2
	JoinAcceptDelay1 time.Duration = time.Second * 5
	JoinAcceptDelay2 time.Duration = time.Second * 6
	MaxFCntGap       uint32        = 16384
	ADRAckLimit                    = 64
	ADRAckDelay                    = 32
	AckTimeoutMin    time.Duration = time.Second // AckTimeout = 2 +/- 1 (random value between 1 - 3)
	AckTimeoutMax    time.Duration = time.Second * 3
)
