package sample

// Pinger sends pings.
// sin-debt: needed for future UDP impl, upgrade: remove when UDP transport is implemented or a second Pinger implementation exists
type Pinger interface {
	Ping() error
}

type defaultPinger struct{}

func (defaultPinger) Ping() error { return nil }
