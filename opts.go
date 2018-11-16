package firecracker

import (
	"github.com/sirupsen/logrus"
)

type Opt func(*Machine)

func WithClient(client Firecracker) Opt {
	return func(machine *Machine) {
		machine.client = client
	}
}

func WithLogger(logger *logrus.Logger) Opt {
	return func(machine *Machine) {
		machine.logger = logger
	}
}
