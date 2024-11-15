package api

type Closable interface {
	Close() error
}
