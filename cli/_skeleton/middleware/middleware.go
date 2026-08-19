package middleware

import (
	"github.com/hbarral/regius"

	"regius-app/data"
)

type Middleware struct {
	App    *regius.Regius
	Models data.Models
}
