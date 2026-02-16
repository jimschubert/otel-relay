package proxy

type Proxy interface {
	Protocol() string
	Start() error
	Stop() error
	Err() error
}
