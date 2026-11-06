package asciidoctor

import (
	"context"
	"errors"
	"sync"
)

// Pool is a fixed set of Asciidoctor instances for converting in parallel.
// Each call borrows an idle instance, waiting for one if all are busy.
//
// Instances are independent interpreters: configure each one the same way
// (extensions, converters, log handlers) with the setup function passed to
// NewPool.
type Pool struct {
	idle      chan *Asciidoctor
	all       []*Asciidoctor
	closeOnce sync.Once
	closeErr  error
}

// NewPool starts size instances with opts and runs setup, if non-nil, on each.
func NewPool(size int, setup func(*Asciidoctor) error, opts ...Option) (*Pool, error) {
	if size < 1 {
		return nil, errors.New("asciidoctor: pool size must be at least 1")
	}
	p := &Pool{idle: make(chan *Asciidoctor, size)}
	for i := 0; i < size; i++ {
		a, err := New(opts...)
		if err == nil && setup != nil {
			if err = setup(a); err != nil {
				a.Close()
			}
		}
		if err != nil {
			p.Close()
			return nil, err
		}
		p.all = append(p.all, a)
		p.idle <- a
	}
	return p, nil
}

// Do runs fn with an instance borrowed from the pool.
func (p *Pool) Do(fn func(*Asciidoctor) error) error {
	return p.DoContext(context.Background(), fn)
}

// DoContext runs fn with an instance borrowed from the pool. While all
// instances are busy it waits until one is idle or ctx is done, and then
// returns ctx.Err().
func (p *Pool) DoContext(ctx context.Context, fn func(*Asciidoctor) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var a *Asciidoctor
	select {
	case a = <-p.idle:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { p.idle <- a }()
	return fn(a)
}

// Convert converts source with a pooled instance; see Asciidoctor.Convert.
func (p *Pool) Convert(source string, options *Options) (string, error) {
	return p.ConvertContext(context.Background(), source, options)
}

// ConvertContext converts source with a pooled instance; see
// Asciidoctor.ConvertContext. ctx also bounds the wait for an idle instance.
func (p *Pool) ConvertContext(ctx context.Context, source string, options *Options) (out string, err error) {
	err = p.DoContext(ctx, func(a *Asciidoctor) error {
		out, err = a.ConvertContext(ctx, source, options)
		return err
	})
	return out, err
}

// ConvertFile converts a file with a pooled instance; see
// Asciidoctor.ConvertFile.
func (p *Pool) ConvertFile(path string, options *Options) (string, error) {
	return p.ConvertFileContext(context.Background(), path, options)
}

// ConvertFileContext converts a file with a pooled instance; see
// Asciidoctor.ConvertFileContext. ctx also bounds the wait for an idle
// instance.
func (p *Pool) ConvertFileContext(ctx context.Context, path string, options *Options) (out string, err error) {
	err = p.DoContext(ctx, func(a *Asciidoctor) error {
		out, err = a.ConvertFileContext(ctx, path, options)
		return err
	})
	return out, err
}

// Close closes every instance, waiting for a call in progress on an instance
// to finish. Later calls fail with ErrClosed.
func (p *Pool) Close() error {
	p.closeOnce.Do(func() {
		var errs []error
		for _, a := range p.all {
			if err := a.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		p.closeErr = errors.Join(errs...)
	})
	return p.closeErr
}
