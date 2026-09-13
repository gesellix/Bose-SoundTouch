//go:build linux && arm

package netcompat

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// defaultKeepAlivePeriod matches what net.TCPListener.Accept applies to the
// connections it returns. net.FileConn does not inherit it, and this service
// holds long-lived websockets to speakers, so the raw accept path re-applies it
// rather than handing back a connection with keep-alives silently disabled.
const defaultKeepAlivePeriod = 15 * time.Second

// Listen is net.Listen plus a fallback for kernels without accept4().
//
// Non-TCP listeners are returned untouched: the service's UDP listeners never
// accept, and no other network type is used here.
func Listen(network, address string) (net.Listener, error) {
	ln, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}

	tcpLn, ok := ln.(*net.TCPListener)
	if !ok {
		return ln, nil
	}

	switch requestedFallback() {
	case fallbackForceOff:
		return ln, nil
	case fallbackForceOn:
		log.Printf("[netcompat] %s=1: accepting connections on %s without accept4()", FallbackEnv, ln.Addr())

		return newFallbackListener(tcpLn, true), nil
	default:
		// Probe eagerly so a kernel without accept4() is reported at startup
		// rather than when the first client shows up and the connection is
		// already in flight. The latch inside the listener stays as a backstop
		// in case the probe could not run.
		if !accept4Available() {
			log.Printf("[netcompat] This kernel has no accept4(); accepting connections on %s through accept() instead", ln.Addr())

			return newFallbackListener(tcpLn, true), nil
		}

		return newFallbackListener(tcpLn, false), nil
	}
}

// accept4Available reports whether the running kernel implements accept4().
//
// The probe is a throwaway loopback listener with an empty accept queue, so a
// kernel that has the syscall answers EAGAIN and one that does not answers
// ENOSYS. Nothing is consumed either way. If the probe itself cannot be set up
// we answer "available", which leaves the standard path in place and defers to
// the ENOSYS latch.
func accept4Available() bool {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return true
	}
	defer func() { _ = ln.Close() }()

	tcpLn, ok := ln.(*net.TCPListener)
	if !ok {
		return true
	}

	rc, err := tcpLn.SyscallConn()
	if err != nil {
		return true
	}

	var errno syscall.Errno

	if err := rc.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall6(syscall.SYS_ACCEPT4, fd, 0, 0, 0, 0, 0)
	}); err != nil {
		return true
	}

	return errno != syscall.ENOSYS
}

// fallbackListener accepts through accept4() until that syscall turns out to be
// missing, then through accept() for the rest of its life.
//
// The TCPListener is a named field rather than embedded, so that "the standard
// library's accept" and "our accept" never read alike at a call site.
type fallbackListener struct {
	inner *net.TCPListener

	// raw is set once, either by the startup probe or by the first ENOSYS.
	raw atomic.Bool
}

func newFallbackListener(ln *net.TCPListener, raw bool) *fallbackListener {
	l := &fallbackListener{inner: ln}
	l.raw.Store(raw)

	return l
}

func (l *fallbackListener) Close() error { return l.inner.Close() }

func (l *fallbackListener) Addr() net.Addr { return l.inner.Addr() }

func (l *fallbackListener) Accept() (net.Conn, error) {
	if !l.raw.Load() {
		conn, err := l.inner.AcceptTCP()
		if err == nil {
			return conn, nil
		}

		if !errors.Is(err, syscall.ENOSYS) {
			return nil, err
		}

		// internal/poll's accept loop retries only on EINTR, EAGAIN and
		// ECONNABORTED, so ENOSYS came straight back out and the pending
		// connection is still queued for the raw accept below.
		if l.raw.CompareAndSwap(false, true) {
			log.Printf("[netcompat] accept4() is not implemented on this kernel; falling back to accept() on %s", l.Addr())
		}
	}

	return l.acceptRaw()
}

// acceptWaitTimeout bounds each poll(2) wait in acceptRaw. The wait runs inside
// rc.Control, which holds a reference on the listening fd, so Close cannot
// finish until the wait returns. This is the most Close can be delayed by.
const acceptWaitTimeout = 250 * time.Millisecond

// acceptRaw accepts on the raw listening fd, waiting for it to become readable
// with poll(2).
//
// It cannot park in netpoll the way the standard library does: the RawConn of a
// listener refuses Read and Write with a bare EINVAL (net.rawListener), which
// is what issue 698's first fallback ran into on real hardware. Control is the
// only RawConn method a listener allows, so each round accepts once and, if the
// queue is empty, waits a bounded time for the next connection.
func (l *fallbackListener) acceptRaw() (net.Conn, error) {
	rc, err := l.inner.SyscallConn()
	if err != nil {
		return nil, err
	}

	for {
		var (
			nfd       int
			acceptErr error
		)

		if err := rc.Control(func(fd uintptr) {
			nfd, acceptErr = rawAccept(int(fd))
			if errors.Is(acceptErr, syscall.EAGAIN) {
				waitReadable(int(fd), acceptWaitTimeout)
			}
		}); err != nil {
			// The listener was closed; err wraps net.ErrClosed.
			return nil, err
		}

		switch {
		case acceptErr == nil:
			return connFromFD(nfd, l.Addr())
		case errors.Is(acceptErr, syscall.EAGAIN),
			errors.Is(acceptErr, syscall.EINTR),
			errors.Is(acceptErr, syscall.ECONNABORTED):
			// Nothing was handed to us: the queue was empty, or a client gave
			// up before we got to it. Try again.
			continue
		default:
			return nil, &net.OpError{Op: "accept", Net: l.Addr().Network(), Addr: l.Addr(), Err: acceptErr}
		}
	}
}

// waitReadable blocks until fd is readable or timeout passes. Its result is
// deliberately ignored: the caller accepts again either way, and accept(2)
// reports anything that matters.
func waitReadable(fd int, timeout time.Duration) {
	pfd := struct {
		fd      int32
		events  int16
		revents int16
	}{fd: int32(fd), events: pollIn}

	_, _, _ = syscall.Syscall(syscall.SYS_POLL, uintptr(unsafe.Pointer(&pfd)), 1, uintptr(timeout.Milliseconds()))
}

// pollIn is POLLIN, which package syscall does not define.
const pollIn = 0x1

// rawAccept calls accept(2) directly. syscall.Accept cannot be used: on Linux
// it is defined as Accept4(fd, 0), which is the syscall we are working around.
//
// accept(2) has no SOCK_CLOEXEC, so the fd is made close-on-exec under ForkLock
// the way the standard library did before accept4() existed, keeping a
// concurrent fork from inheriting the connection.
func rawAccept(fd int) (int, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()

	nfd, _, errno := syscall.Syscall(syscall.SYS_ACCEPT, uintptr(fd), 0, 0)
	if errno != 0 {
		return -1, errno
	}

	syscall.CloseOnExec(int(nfd))

	return int(nfd), nil
}

// connFromFD turns an accepted fd into a net.Conn.
//
// net.FileConn duplicates the descriptor, so the original is closed here rather
// than leaked. The duplicate is what the caller gets, with keep-alives applied
// to match a connection from net.TCPListener.Accept.
func connFromFD(nfd int, addr net.Addr) (net.Conn, error) {
	f := os.NewFile(uintptr(nfd), "netcompat-accept")
	if f == nil {
		return nil, &net.OpError{Op: "accept", Net: addr.Network(), Addr: addr, Err: fmt.Errorf("invalid descriptor %d", nfd)}
	}

	defer func() { _ = f.Close() }()

	conn, err := net.FileConn(f)
	if err != nil {
		return nil, err
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(defaultKeepAlivePeriod)
	}

	return conn, nil
}
