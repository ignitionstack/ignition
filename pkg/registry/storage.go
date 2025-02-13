package registry

type Storage interface {
	ReadWASMFile(path string) ([]byte, error)
	WriteWASMFile(path string, data []byte) error
	BuildWASMPath(namespace, name, shortDigest string) string
}
