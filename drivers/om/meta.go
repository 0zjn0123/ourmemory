package omdriver

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

// Addition intentionally has no user-facing options. The shared/private
// layout is fixed so the file browser and the community pages cannot drift.
type Addition struct{}

var config = driver.Config{
	Name:        "OM",
	LocalSort:   true,
	NoCache:     true,
	DefaultRoot: "/",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &OM{}
	})
}
