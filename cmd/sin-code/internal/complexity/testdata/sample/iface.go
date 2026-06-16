package sample

// Pinger sends pings.
// sin-debt: needed for future UDP impl
type Pinger interface {
	Ping() error
}

type defaultPinger struct{}

func (defaultPinger) Ping() error { return nil }
