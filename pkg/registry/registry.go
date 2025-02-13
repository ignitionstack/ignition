package registry

type Registry interface {
	Get(namespace, name string) (*FunctionMetadata, error)
	Push(namespace, name string, payload []byte, digest, tag string) error
	Pull(namespace, name string, version string) ([]byte, string, error)
	ReassignTag(namespace, name, tag, newDigest string) error
	DigestExists(namespace, name, digest string) (bool, error)
	ListAll() ([]FunctionMetadata, error)
}
