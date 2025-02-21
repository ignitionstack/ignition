package builders

type Builder interface {
	Build(path string) (*BuildResult, error)
	VerifyDependencies() error
}

type BuildResult struct {
	OutputPath string
}

type BuildError struct {
	Err    error
	Stderr string
	Step   string
}

func (e *BuildError) Error() string {
	if e.Stderr != "" {
		return e.Stderr
	}
	return e.Err.Error()
}
