package builders

type Builder interface {
	Build(path string) (*BuildResult, error)
}

type BuildResult struct {
	OutputPath string
}
