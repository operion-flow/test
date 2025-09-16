package actions

import "github.com/operion-flow/test/integration/environment"

type Action interface {
	Execute(env *environment.Environment) error
	Description() string
}
